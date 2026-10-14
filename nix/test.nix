# End-to-end test of services.mayuri2000 in a NixOS VM (checks.nixos-module).
# The VM has no internet: a local DNS server sends discord.com to a fake
# public address whose listener never answers, so the bot starts, gets stuck
# connecting, and stays up long enough to be inspected.
{
  pkgs,
  quadletModule,
  mayuriModule,
}:
let
  fakeToken = "MTIzNDU2Nzg5MDEyMzQ1Njc4.fake.vmtesttokenvalue";
  lanIP = "192.168.1.1"; # the test node's private address
  discordIP = "203.0.113.10"; # TEST-NET-3: public, so not blocked
in
pkgs.testers.runNixOSTest {
  name = "mayuri2000-module";

  nodes.machine =
    { pkgs, ... }:
    {
      imports = [
        quadletModule
        mayuriModule
      ];
      virtualisation = {
        memorySize = 3072;
        diskSize = 4096;
      };

      environment.etc."mayuri-token" = {
        text = fakeToken;
        mode = "0400";
        user = "mayuri2000";
        group = "mayuri2000";
      };
      services.mayuri2000 = {
        enable = true;
        tokenFile = "/etc/mayuri-token";
        environment.LOG_LEVEL = "debug";
      };

      # Resolver on the private network: allowed only via podman's DNS on the
      # container network's gateway, which forwards here from the host.
      services.resolved.enable = false;
      networking.nameservers = [ lanIP ];
      services.dnsmasq = {
        enable = true;
        resolveLocalQueries = false;
        settings = {
          listen-address = lanIP;
          bind-interfaces = true;
          no-resolv = true;
          address = "/discord.com/${discordIP}";
        };
      };
      # Keep the host firewall on, with the nftables backend: there podman's own
      # accept rules live in a separate table and can't override NixOS's drop,
      # so the module must open DNS on its network itself (with iptables,
      # podman's rules land in the same chain and hide a missing rule). The
      # test servers below are reached from the container.
      networking.nftables.enable = true;
      networking.firewall.allowedTCPPorts = [
        443
        8000
        8080
      ];

      # A "public" Discord that accepts connections and never answers, and a
      # LAN service the container must not reach.
      systemd.services.fake-discord = {
        wantedBy = [ "multi-user.target" ];
        after = [ "network.target" ];
        path = [ pkgs.iproute2 ];
        preStart = ''
          ip link add fake0 type dummy || true
          ip addr add ${discordIP}/32 dev fake0 || true
          ip link set fake0 up
        '';
        script = ''
          exec ${pkgs.python3}/bin/python3 -c '
          import socket, time
          s = socket.socket(); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
          s.bind(("${discordIP}", 443)); s.listen(64); conns = []
          while True: conns.append(s.accept()[0])
          '
        '';
      };
      systemd.services.public-http = {
        wantedBy = [ "multi-user.target" ];
        after = [ "fake-discord.service" ];
        requires = [ "fake-discord.service" ];
        script = "exec ${pkgs.python3}/bin/python3 -m http.server --bind ${discordIP} 8080 --directory /etc";
      };
      systemd.services.lan-http = {
        wantedBy = [ "multi-user.target" ];
        after = [ "network-online.target" ];
        wants = [ "network-online.target" ];
        script = "exec ${pkgs.python3}/bin/python3 -m http.server --bind ${lanIP} 8000 --directory /etc";
      };
    };

  testScript = ''
    import json

    machine.wait_for_unit("multi-user.target")
    machine.wait_for_unit("lan-http.service")
    machine.wait_for_unit("fake-discord.service")
    machine.wait_for_unit("mayuri2000.service")

    with subtest("decoder sandbox and database"):
        machine.wait_until_succeeds("journalctl -u mayuri2000 | grep -q 'decoder sandbox active'", timeout=240)
        machine.succeed("test \"$(stat -c '%u:%g %a' /var/lib/mayuri2000)\" = '10001:10001 700'")
        machine.wait_until_succeeds("test \"$(stat -c %u /var/lib/mayuri2000/mayuri.db)\" = 10001", timeout=60)

    with subtest("hardened container"):
        machine.wait_until_succeeds("test \"$(podman inspect -f '{{.State.Running}}' mayuri2000)\" = true", timeout=120)
        info = json.loads(machine.succeed("podman inspect mayuri2000"))[0]
        cfg, host = info["Config"], info["HostConfig"]
        assert cfg["User"] == "10001:10001", cfg["User"]
        assert host["ReadonlyRootfs"], "root filesystem is writable"
        assert info["EffectiveCaps"] in (None, []), info["EffectiveCaps"]
        assert "no-new-privileges" in host["SecurityOpt"], host["SecurityOpt"]
        assert "seccomp=unconfined" not in host["SecurityOpt"], host["SecurityOpt"]
        assert host["PidsLimit"] == 512, host["PidsLimit"]
        assert host["Memory"] == 3 * 1024**3, host["Memory"]
        pid = info["State"]["Pid"]
        machine.succeed(f"test \"$(stat -c %u /proc/{pid})\" = 10001")

    with subtest("token only as a file"):
        env = cfg["Env"]
        assert "DISCORD_TOKEN_FILE=/run/secrets/discord_token" in env, env
        assert not any(e.startswith("DISCORD_TOKEN=") for e in env), env
        assert "${fakeToken}" not in json.dumps(info), "token value in podman inspect"
        machine.fail("grep -rqs '${fakeToken}' /etc/containers /run/systemd /etc/systemd")
        pid = json.loads(machine.succeed("podman inspect mayuri2000"))[0]["State"]["Pid"]
        machine.fail(f"tr '\\0' '\\n' < /proc/{pid}/environ | grep -q '${fakeToken}'")

    with subtest("DNS through the container network works"):
        # The bot resolved discord.com (via podman's DNS -> dnsmasq on the
        # LAN) and is stuck connecting to the fake Discord.
        machine.wait_until_succeeds("ss -tn state established '( sport = :443 )' | grep -q ${discordIP}", timeout=120)
        machine.fail("journalctl -u mayuri2000 | grep -q 'no such host'")

    with subtest("LAN is unreachable from the container"):
        # systemd's IP filter drops packets, so a blocked connection times out.
        # Fetching a text file makes ffmpeg fail with "Invalid data" instead
        # when the connection did work.
        machine.wait_for_unit("public-http.service")
        machine.succeed("curl -sf http://${lanIP}:8000/hostname")
        ffmpeg = next(e.split("=", 1)[1] for e in cfg["Env"] if e.startswith("FFMPEG_PATH="))

        def fetch(url):
            return machine.fail(f"podman exec mayuri2000 {ffmpeg} -hide_banner -rw_timeout 5000000 -i {url} -f null - 2>&1")

        reached = "Invalid data found"
        out = fetch("http://${discordIP}:8080/hostname")
        assert reached in out, f"public address should be reachable: {out}"
        out = fetch("http://${lanIP}:8000/hostname")
        assert reached not in out and "timed out" in out, f"LAN should be blocked: {out}"

        # Control: without the filter the same request works, so the filter
        # (not routing) is what blocks it.
        machine.succeed("systemctl set-property --runtime mayuri2000.service IPAddressDeny=")
        out = fetch("http://${lanIP}:8000/hostname")
        assert reached in out, f"LAN should be reachable without the filter: {out}"
  '';
}
