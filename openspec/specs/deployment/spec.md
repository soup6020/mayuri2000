# deployment Specification

## Purpose
How the bot is configured, built, verified and run: environment configuration, persistence, the Nix build and OCI image, quality gates, and supported deployment methods.

## Requirements

### Requirement: Configuration
The bot SHALL be configured only through environment variables: `DISCORD_TOKEN` (required), `BOT_PREFIX` (default `%`), `DATABASE_PATH`, `DEV_GUILD_IDS` (guild-only command sync for testing), `TENOR_API_KEY`, `GENIUS_TOKEN`, `MAGICK_PATH`, `RSS_INTERVAL` (default 5m) and `LOG_LEVEL`. Empty values SHALL behave as unset; invalid values MUST stop startup with a clear error.

#### Scenario: Missing token
- **WHEN** the bot starts without `DISCORD_TOKEN`
- **THEN** it exits saying DISCORD_TOKEN or DISCORD_TOKEN_FILE must be set

### Requirement: Persistence
All state (prefixes, aliases, RSS feeds, reminders, user time zones, all-time stats) SHALL be stored in one SQLite database whose schema is created and upgraded automatically at startup. In the container image the database SHALL live under `/data`.

#### Scenario: Upgrade
- **WHEN** a new version adds a table
- **THEN** it is created on startup without manual migration

### Requirement: Nix build
The project SHALL be built with a flake-parts flake (never flake-utils) providing `packages.default` (static binary, CGO disabled, whole test suite run in the check phase), `packages.image` (OCI image with the bot, ImageMagick with liblqr, CA certificates and tzdata) and a devShell with Go, gopls, golangci-lint, ImageMagick and sqlite.

#### Scenario: Build the image
- **WHEN** a developer runs `nix build .#image`
- **THEN** an OCI archive is produced that `docker load` or `podman load` accepts

### Requirement: Quality gates
`nix flake check` SHALL build and test the package and run golangci-lint using `.golangci.yml`; changes MUST pass it with 0 lint issues. Third-party API and scraped-page parsers SHALL be tested against saved fixtures, and ImageMagick tests SHALL skip when magick is unavailable.

#### Scenario: Lint failure
- **WHEN** a change introduces an unchecked error
- **THEN** `nix flake check` fails

### Requirement: Deployment methods
The primary deployment SHALL be the OCI image run by quadlet-nix. A Docker Compose example (`compose.yaml`) SHALL read configuration from `.env` (template `env.example`) and keep data in a named volume.

#### Scenario: Compose
- **WHEN** a user copies `env.example` to `.env`, sets the token and runs `docker compose up -d`
- **THEN** the bot starts with its database in the `mayuri-data` volume
