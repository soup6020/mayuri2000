# deployment Specification

## Purpose
How the bot is configured, built, verified and run: environment configuration, persistence, the Nix build and OCI image, the hardened container and NixOS module, quality gates, and supported deployment methods.

## Requirements

### Requirement: Configuration
The bot SHALL be configured only through environment variables: `DISCORD_TOKEN` (required), `BOT_PREFIX` (default `%`), `DATABASE_PATH`, `DEV_GUILD_IDS` (guild-only command sync for testing), `TENOR_API_KEY`, `GENIUS_TOKEN`, `MAGICK_PATH`, `FFMPEG_PATH` (default `ffmpeg`), `RSS_INTERVAL` (default 5m), `DECODER_SANDBOX` (`required` by default, or `best-effort`) and `LOG_LEVEL`. Empty values SHALL behave as unset; invalid values MUST stop startup with a clear error.

#### Scenario: Missing token
- **WHEN** the bot starts without `DISCORD_TOKEN`
- **THEN** it exits saying DISCORD_TOKEN or DISCORD_TOKEN_FILE must be set

#### Scenario: Invalid sandbox mode
- **WHEN** the bot starts with `DECODER_SANDBOX=off`
- **THEN** it exits saying DECODER_SANDBOX must be required or best-effort

### Requirement: Persistence
All state (prefixes, aliases, RSS feeds, reminders, user time zones, all-time stats) SHALL be stored in one SQLite database whose schema is created and upgraded automatically at startup. In the container image the database SHALL live under `/data`.

#### Scenario: Upgrade
- **WHEN** a new version adds a table
- **THEN** it is created on startup without manual migration

### Requirement: Nix build
The project SHALL be built with a flake-parts flake (never flake-utils) providing `packages.default` (static binary, CGO disabled, whole test suite run in the check phase), `packages.image` (OCI image with the bot, ImageMagick with liblqr, ffmpeg, CA certificates and tzdata, running as a non-root user) and a devShell with Go, gopls, golangci-lint, ImageMagick, ffmpeg, sqlite and openspec.

#### Scenario: Build the image
- **WHEN** a developer runs `nix build .#image`
- **THEN** an OCI archive is produced that `docker load` or `podman load` accepts

### Requirement: Quality gates
`nix flake check` SHALL build and test the package, run golangci-lint using `.golangci.yml`, and validate the OpenSpec specs strictly; changes MUST pass it with 0 lint issues. Third-party API and scraped-page parsers SHALL be tested against saved fixtures, and ImageMagick tests SHALL skip when magick is unavailable.

#### Scenario: Lint failure
- **WHEN** a change introduces an unchecked error
- **THEN** `nix flake check` fails

#### Scenario: Malformed spec
- **WHEN** a spec requirement has no scenario
- **THEN** `nix flake check` fails

### Requirement: Deployment methods
The primary deployment SHALL be the NixOS module (quadlet-nix, typically deployed with colmena and agenix). A Docker Compose example (`compose.yaml`) SHALL read configuration from `.env` (template `env.example`) and keep data in a named volume.

#### Scenario: Compose
- **WHEN** a user copies `env.example` to `.env`, sets the token and runs `docker compose up -d`
- **THEN** the bot starts with its database in the `mayuri-data` volume

#### Scenario: colmena
- **WHEN** a colmena hive imports quadlet-nix, agenix and this flake's module and enables `services.mayuri2000`
- **THEN** `colmena apply` deploys the hardened container and restarts it when the image changes

### Requirement: Privileged intents
The Discord application SHALL have the Message Content privileged intent enabled (for text-prefix commands) and the Server Members privileged intent enabled (for listing role members). The README MUST document both as setup steps.

#### Scenario: Fresh setup
- **WHEN** someone follows the README to set up the bot
- **THEN** they are told to enable both the Message Content and Server Members intents in the Developer Portal

### Requirement: Unprivileged container
The OCI image SHALL run the bot as the non-root user `mayuri` (UID and GID 10001) with `/data` owned by that user. The bot MUST work with a read-only root filesystem, a writable tmpfs at `/tmp`, all Linux capabilities dropped and no-new-privileges set; it SHALL write only to `/data` and `/tmp`.

#### Scenario: Runs as non-root
- **WHEN** the image is run
- **THEN** the bot process runs as UID 10001, not root

#### Scenario: Hardened runtime
- **WHEN** the container runs with a read-only root filesystem, tmpfs `/tmp`, `--cap-drop=ALL` and `no-new-privileges`
- **THEN** the bot starts, opens its database in `/data`, and ImageMagick and ffmpeg can process images and videos

### Requirement: Hardened deployment examples
The Docker Compose example and the NixOS module SHALL apply: read-only root filesystem, tmpfs `/tmp` with a size limit, all capabilities dropped, no-new-privileges, the runtime's default seccomp profile (never `unconfined`), and process-count and memory limits. The README MUST explain how to give an existing data volume or directory to UID 10001.

#### Scenario: Compose settings
- **WHEN** a user starts the bot with the provided `compose.yaml`
- **THEN** the container has a read-only root filesystem, no capabilities, no-new-privileges, and pids and memory limits

#### Scenario: Upgrading existing data
- **WHEN** an operator upgrades from an image that ran as root
- **THEN** the README gives the one-time command to change the data's owner to 10001

### Requirement: Unwritable data directory
If the directory containing the database is not writable, the bot MUST stop at startup with an error naming the directory and the UID it runs as, and suggesting changing the directory's owner.

#### Scenario: Root-owned volume
- **WHEN** the bot running as UID 10001 starts with a `/data` owned by root
- **THEN** it exits with an error saying `/data` is not writable by UID 10001 and should be chowned

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
