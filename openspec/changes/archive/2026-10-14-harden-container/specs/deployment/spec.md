## ADDED Requirements

### Requirement: Unprivileged container
The OCI image SHALL run the bot as the non-root user `mayuri` (UID and GID 10001) with `/data` owned by that user. The bot MUST work with a read-only root filesystem, a writable tmpfs at `/tmp`, all Linux capabilities dropped and no-new-privileges set; it SHALL write only to `/data` and `/tmp`.

#### Scenario: Runs as non-root
- **WHEN** the image is run
- **THEN** the bot process runs as UID 10001, not root

#### Scenario: Hardened runtime
- **WHEN** the container runs with a read-only root filesystem, tmpfs `/tmp`, `--cap-drop=ALL` and `no-new-privileges`
- **THEN** the bot starts, opens its database in `/data`, and ImageMagick and ffmpeg can process images and videos

### Requirement: Hardened deployment examples
The Docker Compose example and the README quadlet example SHALL apply: read-only root filesystem, tmpfs `/tmp` with a size limit, all capabilities dropped, no-new-privileges, the runtime's default seccomp profile (never `unconfined`), and process-count and memory limits. The README MUST explain how to give an existing data volume or directory to UID 10001.

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

## MODIFIED Requirements

### Requirement: Nix build
The project SHALL be built with a flake-parts flake (never flake-utils) providing `packages.default` (static binary, CGO disabled, whole test suite run in the check phase), `packages.image` (OCI image with the bot, ImageMagick with liblqr, ffmpeg, CA certificates and tzdata, running as a non-root user) and a devShell with Go, gopls, golangci-lint, ImageMagick, ffmpeg, sqlite and openspec.

#### Scenario: Build the image
- **WHEN** a developer runs `nix build .#image`
- **THEN** an OCI archive is produced that `docker load` or `podman load` accepts
