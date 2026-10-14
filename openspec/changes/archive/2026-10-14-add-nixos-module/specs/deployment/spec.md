## ADDED Requirements

### Requirement: NixOS module
The flake SHALL export `nixosModules.default` providing `services.mayuri2000`, which runs the OCI image through quadlet-nix as a rootful podman container with: a host user and group `mayuri2000` (UID/GID 10001), a state directory owned by them mounted at `/data`, the hardened runtime settings, and the decoder sandbox required. A deployment that changes the image SHALL change the container unit so the bot restarts.

#### Scenario: Enable on a host
- **WHEN** a host imports quadlet-nix and this module and sets `services.mayuri2000.enable` and `tokenFile`
- **THEN** the bot runs as UID 10001 with a read-only root, no capabilities and its database in the state directory

#### Scenario: Image update
- **WHEN** the flake input is updated and the host is redeployed with a new image
- **THEN** the container unit changes and the bot restarts on the new image

### Requirement: Secret files in the NixOS module
The module SHALL pass the bot token and optional API keys only as read-only files mounted into the container with `*_FILE` variables, so no secret appears in an environment variable, unit file or the Nix store. Setting `DISCORD_TOKEN`, `GENIUS_TOKEN` or `TENOR_API_KEY` in the module's `environment` MUST fail evaluation.

#### Scenario: agenix token
- **WHEN** `tokenFile` is an agenix secret owned by uid/gid 10001 (the `mayuri2000` user)
- **THEN** the bot reads the token from the mounted file and the container's environment has no token value

#### Scenario: Token in environment
- **WHEN** a host sets `services.mayuri2000.environment.DISCORD_TOKEN`
- **THEN** evaluation fails telling them to use `tokenFile`

### Requirement: Network restriction
With `restrictNetwork` enabled (the default), the container MUST NOT be able to open connections to loopback, private, link-local or carrier-grade NAT addresses, except the container network's DNS server; public internet access SHALL keep working.

#### Scenario: LAN service
- **WHEN** a process in the bot's container connects to a service on the host's private network
- **THEN** the connection fails because the host drops its packets, while connections to public addresses still work

## MODIFIED Requirements

### Requirement: Deployment methods
The primary deployment SHALL be the NixOS module (quadlet-nix, typically deployed with colmena and agenix). A Docker Compose example (`compose.yaml`) SHALL read configuration from `.env` (template `env.example`) and keep data in a named volume.

#### Scenario: Compose
- **WHEN** a user copies `env.example` to `.env`, sets the token and runs `docker compose up -d`
- **THEN** the bot starts with its database in the `mayuri-data` volume

#### Scenario: colmena
- **WHEN** a colmena hive imports quadlet-nix, agenix and this flake's module and enables `services.mayuri2000`
- **THEN** `colmena apply` deploys the hardened container and restarts it when the image changes

### Requirement: Hardened deployment examples
The Docker Compose example and the NixOS module SHALL apply: read-only root filesystem, tmpfs `/tmp` with a size limit, all capabilities dropped, no-new-privileges, the runtime's default seccomp profile (never `unconfined`), and process-count and memory limits. The README MUST explain how to give an existing data volume or directory to UID 10001.

#### Scenario: Compose settings
- **WHEN** a user starts the bot with the provided `compose.yaml`
- **THEN** the container has a read-only root filesystem, no capabilities, no-new-privileges, and pids and memory limits

#### Scenario: Upgrading existing data
- **WHEN** an operator upgrades from an image that ran as root
- **THEN** the README gives the one-time command to change the data's owner to 10001
