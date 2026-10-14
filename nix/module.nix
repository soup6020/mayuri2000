# NixOS module running Mayuri2000's OCI image as a hardened quadlet-nix
# (rootful podman) container. Secrets are only ever passed as files; see the
# deployment spec. Hosts must also import quadlet-nix's NixOS module.
{ withSystem }:
{
  config,
  lib,
  options,
  pkgs,
  ...
}:
let
  cfg = config.services.mayuri2000;
  inherit (lib) mkOption types;

  # The image runs as this fixed uid/gid; the host user must match.
  id = 10001;

  secretNames = [
    "DISCORD_TOKEN"
    "GENIUS_TOKEN"
    "TENOR_API_KEY"
  ];
  secretFiles = lib.filterAttrs (_: f: f != null) {
    DISCORD_TOKEN = cfg.tokenFile;
    GENIUS_TOKEN = cfg.geniusTokenFile;
    TENOR_API_KEY = cfg.tenorApiKeyFile;
  };
  # Inside the container each secret is a read-only file in /run/secrets.
  secretPath = name: "/run/secrets/${lib.toLower name}";

  forbiddenEnv = lib.filter (n: cfg.environment ? ${n} || cfg.environment ? "${n}_FILE") secretNames;

  secretFileOption =
    description:
    mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "/run/agenix/mayuri-genius";
      inherit description;
    };
in
{
  options.services.mayuri2000 = {
    enable = lib.mkEnableOption "the Mayuri2000 Discord bot (hardened quadlet-nix container)";

    image = mkOption {
      type = types.package;
      default = withSystem pkgs.stdenv.hostPlatform.system ({ config, ... }: config.packages.image);
      defaultText = lib.literalMD "the image built by this flake for the host's system";
      description = "OCI image archive to run (a `dockerTools` image).";
    };

    tokenFile = mkOption {
      type = types.str;
      example = "/run/agenix/mayuri-token";
      description = ''
        File containing only the Discord bot token, readable by the `mayuri2000`
        user (uid 10001). A string, so the secret is never copied into the Nix
        store; with agenix use `config.age.secrets.<name>.path` and set the
        secret's owner to `mayuri2000`.
      '';
    };
    geniusTokenFile = secretFileOption "Optional file containing a Genius API token.";
    tenorApiKeyFile = secretFileOption "Optional file containing a Tenor API key.";

    environment = mkOption {
      type = types.attrsOf types.str;
      default = { };
      example = {
        BOT_PREFIX = "%";
        LOG_LEVEL = "info";
      };
      description = ''
        Non-secret configuration (`BOT_PREFIX`, `DEV_GUILD_IDS`, `LOG_LEVEL`,
        `RSS_INTERVAL`, `DECODER_SANDBOX`). Secrets belong in the `*File` options.
      '';
    };

    dataDir = mkOption {
      type = types.str;
      default = "/var/lib/mayuri2000";
      description = "Host directory holding the SQLite database, mounted at /data.";
    };

    memory = mkOption {
      type = types.str;
      default = "3g";
      description = "Container memory limit (includes the /tmp tmpfs).";
    };
    pidsLimit = mkOption {
      type = types.ints.positive;
      default = 512;
      description = "Maximum number of processes and threads in the container.";
    };
    tmpSize = mkOption {
      type = types.str;
      default = "1g";
      description = "Size of the /tmp tmpfs used for image and video jobs.";
    };

    restrictNetwork = mkOption {
      type = types.bool;
      default = true;
      description = ''
        Deny connections from the container to loopback, private, link-local,
        CGNAT and multicast addresses (systemd IPAddressDeny on the container's
        unit), so the bot can't reach the host's LAN. The container's own
        network gateway stays allowed for DNS.
      '';
    };
    subnet = mkOption {
      type = types.str;
      default = "10.89.173.0/24";
      description = "Subnet of the container's dedicated podman network.";
    };
    gateway = mkOption {
      type = types.str;
      default = "10.89.173.1";
      description = "Gateway (and DNS server) address in `subnet`.";
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = options ? virtualisation.quadlet;
        message = "services.mayuri2000 needs quadlet-nix: add quadlet-nix.nixosModules.quadlet to your imports.";
      }
      {
        assertion = forbiddenEnv == [ ];
        message = "services.mayuri2000.environment must not contain ${lib.concatStringsSep ", " forbiddenEnv}; use tokenFile, geniusTokenFile or tenorApiKeyFile so secrets stay out of the environment and the Nix store.";
      }
    ];

    users.users.mayuri2000 = {
      isSystemUser = true;
      uid = id;
      group = "mayuri2000";
      description = "Mayuri2000 Discord bot (container uid)";
    };
    users.groups.mayuri2000.gid = id;

    systemd.tmpfiles.rules = [ "d ${cfg.dataDir} 0700 mayuri2000 mayuri2000 -" ];

    # A dedicated network with podman's DNS server on the gateway: the only
    # private address the network restriction has to allow. (The default
    # network has no DNS and hands containers the host's resolvers, often a
    # LAN router.)
    virtualisation.quadlet.networks.mayuri2000.networkConfig = {
      subnets = [ cfg.subnet ];
      gateways = [ cfg.gateway ];
    };

    virtualisation.quadlet.containers.mayuri2000 = {
      autoStart = true;
      # cgroup BPF filters apply to every socket opened in the unit's cgroup,
      # which includes the container, whatever its network namespace.
      serviceConfig = lib.mkIf cfg.restrictNetwork {
        IPAddressDeny = "localhost link-local multicast 0.0.0.0/8 10.0.0.0/8 100.64.0.0/10 172.16.0.0/12 192.168.0.0/16 fc00::/7";
        IPAddressAllow = cfg.gateway;
      };
      containerConfig = {
        networks = [ config.virtualisation.quadlet.networks.mayuri2000.ref ];
        # The store path changes with every new image, so a redeploy with a
        # new image changes this unit and restarts the bot.
        image = "docker-archive:${cfg.image}";
        volumes = [
          "${cfg.dataDir}:/data"
        ]
        ++ lib.mapAttrsToList (name: file: "${file}:${secretPath name}:ro") secretFiles;
        environments =
          lib.mapAttrs' (name: _: lib.nameValuePair "${name}_FILE" (secretPath name)) secretFiles
          // cfg.environment;

        # Hardening, as in compose.yaml (see the deployment spec).
        readOnly = true;
        readOnlyTmpfs = false; # /tmp is mounted below with a size cap instead
        tmpfses = [ "/tmp:size=${cfg.tmpSize},mode=1777" ];
        dropCapabilities = [ "all" ];
        noNewPrivileges = true;
        pidsLimit = cfg.pidsLimit;
        memory = cfg.memory;
      };
    };
  };
}
