# Design

## Context

- `packages.image` is a `buildLayeredImage` tarball. It runs as 10001:10001 and expects:
  - `/data` writable;
  - `/tmp` as a tmpfs;
  - everything else read-only.
- quadlet-nix (checked earlier against its source) maps `containerConfig` attributes to quadlet keys: `readOnly`, `readOnlyTmpfs`, `tmpfses`, `dropCapabilities`, `noNewPrivileges`, `pidsLimit`, `memory`, `volumes`, `environments`, `environmentFiles`.
- An `Image=docker-archive:<store path>` reference evaluated correctly into a unit, so podman loads the image straight from the Nix store.
- The bot already reads `<NAME>_FILE` for every secret.
- The deployment host is NixOS, deployed with colmena; secrets come from agenix.

See proposal.md for motivation.

## Goals / Non-Goals

**Goals:**
- One `services.mayuri2000.enable = true` gives the hardened container. Hosts can't weaken it by accident; deliberate overrides through quadlet-nix remain possible.
- No secret ever passes through an environment variable, a unit file or the Nix store.
- Redeploying with a new image restarts the bot. Nothing restarts when nothing changed.
- The module is verified end to end in a VM, not only evaluated.

**Non-Goals:**
- Rootless podman (quadlet-nix through home-manager). System containers fit colmena and agenix better, and the user confirmed rootful is fine.
- A native systemd service without a container. It was discussed as an alternative, but the user chose quadlet-nix.
- User-namespace remapping (`UserNS=`/`UIDMap=`), so that container UID 10001 maps to an unused high host UID. It's possible later hardening. Host UID 10001 belongs to a dedicated user that owns only the state directory and secrets.
- Colmena or agenix configuration itself. The README shows the hive side; this repo can't test the user's hive.

## Decisions

### Module wiring (flake-parts)

- `flake.nixosModules.default = importApply ./nix/module.nix { inherit withSystem; }`.
- The module's `image` option defaults to `withSystem pkgs.stdenv.hostPlatform.system ({ config, ... }: config.packages.image)`, so hosts get the image built from the flake revision they pinned.
- The module doesn't import quadlet-nix itself; a host would otherwise import it twice. Instead it asserts that `options.virtualisation ? quadlet` and tells the user to add `quadlet-nix.nixosModules.quadlet`.
- `quadlet-nix` becomes a flake input, used only by the VM test.

### Options

- `enable`
- `image` (package, default above)
- `tokenFile` (str, required)
- `geniusTokenFile`, `tenorApiKeyFile` (nullOr str)
- `environment` (attrsOf str) for `BOT_PREFIX`, `DEV_GUILD_IDS`, `LOG_LEVEL`, `RSS_INTERVAL`, `DECODER_SANDBOX`
- `dataDir` (default `/var/lib/mayuri2000`)
- `memory` (default `"3g"`), `pidsLimit` (512), `tmpSize` (`"1g"`)
- `restrictNetwork` (default true)

The paths are strings, not Nix paths, so secret files are never copied into the store.

### Generated configuration

- `users.users.mayuri2000` (system user, `uid = 10001`, no shell or home) and `users.groups.mayuri2000` (`gid = 10001`). The fixed IDs must match the image, and NixOS's duplicate-ID assertion catches clashes.
- `systemd.tmpfiles.rules`: `d <dataDir> 0700 mayuri2000 mayuri2000 -`. The README documents chowning existing data.
- `virtualisation.quadlet.containers.mayuri2000`:
  - `image = "docker-archive:${image}"`;
  - `volumes`: `<dataDir>:/data` and each secret file `:/run/secrets/<name>:ro`;
  - `environments`: the `*_FILE` paths plus `environment`;
  - `readOnly = true`, `readOnlyTmpfs = false`, `tmpfses = [ "/tmp:size=<tmpSize>,mode=1777" ]`;
  - `dropCapabilities = [ "all" ]`, `noNewPrivileges = true`;
  - `pidsLimit`, `memory`;
  - `autoStart = true`.
- An assertion rejects `DISCORD_TOKEN`, `GENIUS_TOKEN` and `TENOR_API_KEY` (and the matching `_FILE` names) in `environment`.

### Restart on update

The unit text contains the image's store path, so every image change changes the unit. NixOS `switch-to-configuration` restarts changed units, which colmena relies on. No tag scheme is needed.

Secret rotation: agenix paths stay the same and the bind mount keeps the old file until the container restarts. The README tells you to run `systemctl restart mayuri2000` after rotating a token. Automatic restart on secret change would mean coupling the module to agenix, so it's left out.

### Network restriction via systemd IP filtering

- quadlet runs the container inside its service's cgroup.
- systemd's `IPAddressDeny=`/`IPAddressAllow=` attach cgroup-BPF filters that apply to every socket created by processes in that cgroup subtree, whichever network namespace they're in.
- The module sets these on the quadlet service:
  - `IPAddressDeny=localhost link-local multicast 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 100.64.0.0/10 fc00::/7`;
  - `IPAddressAllow=` the podman network's DNS and gateway address (default network `10.88.0.1`).
- Public addresses stay allowed. This complements `safehttp`, which already refuses these ranges for user URLs, by enforcing it outside the process.

Alternatives considered:
- nftables rules per container IP: more moving parts, and they depend on podman's network allocation.
- A dedicated podman network with `--internal`: blocks everything, including Discord.

**Risk:** the exact interaction between rootful podman's network setup and cgroup BPF has to be confirmed. The VM test checks that a LAN connection is refused while DNS to the gateway works. If the filter can't be made to work without breaking DNS, the option is dropped and the user is told, rather than shipping a filter that looks active but isn't. The user confirmed that LAN blocking is optional: they may add nftables rules on the host later, so dropping it doesn't block this change.

### VM test (`checks.nixos-module`, x86_64-linux)

- `pkgs.testers.runNixOSTest` with one node importing quadlet-nix and the module, plus a token file containing a fake token. The node has no internet access, so the bot starting and then failing to reach Discord is the expected path.
- Checks:
  - the unit is active;
  - the journal shows `decoder sandbox active`;
  - `<dataDir>/mayuri.db` exists, owned by 10001;
  - `podman inspect` shows user 10001, `ReadonlyRootfs`, `CapDrop` ALL, `no-new-privileges` and the limits;
  - the container's `Config.Env` has `DISCORD_TOKEN_FILE` but no token value, and the token string appears in neither the unit file nor the environment.
- **Network check:**
  - Start an HTTP server on the node's private interface.
  - Run a probe inside the container's cgroup and network namespace (moved into the service's cgroup, then `nsenter -n`).
  - The connection must fail, while connecting to the podman gateway's DNS port is allowed.
  - Implemented: systemd's filter drops packets, so a blocked connection *times out* rather than being refused. The test therefore also checks a public address is reachable. As a control, it lifts the filter at runtime (`systemctl set-property --runtime … IPAddressDeny=`) and checks the same LAN request then succeeds, proving the filter is the cause.
- An evaluation-only test asserts that `environment.DISCORD_TOKEN` fails.

## Risks / Trade-offs

- [Fixed UID 10001 collides with an existing host user] → NixOS's duplicate-UID assertion fails evaluation with a clear message. The UID is fixed because the image is.
- [IP filtering breaks DNS or doesn't apply to the container] → The VM test covers both directions, and the fallback above applies.
- [The VM test makes `nix flake check` slower and needs KVM] → It's limited to x86_64-linux. Builders without KVM can skip it with `--option system-features` defaults, and this machine has `kvm` and `nixos-test`.
- [quadlet-nix option names change upstream] → The VM test evaluates and runs the real module from the pinned input. Hosts that `follows` a newer quadlet-nix would surface a rename at evaluation time.

## Migration Plan

1. In the colmena hive: add the `mayuri2000` and `quadlet-nix` inputs, import both modules plus agenix, and define `age.secrets.mayuri-token` with `owner = "10001"; group = "10001";`. Numeric ids, because agenix can decrypt before NixOS creates the `mayuri2000` user; owning by name then failed with "invalid user" on the real host.
2. Enable `services.mayuri2000` with `tokenFile = config.age.secrets.mayuri-token.path`.
3. `colmena apply`.
4. To move the current Compose data over, stop Compose and copy the volume's `mayuri.db*` into `/var/lib/mayuri2000` (owned by 10001).

Rollback: disable the module and redeploy. The Compose setup is unaffected.
