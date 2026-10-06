# Spec Delta

## MODIFIED Requirements

### Requirement: Configuration
The bot SHALL be configured only through environment variables: `DISCORD_TOKEN` (required), `BOT_PREFIX` (default `%`), `DATABASE_PATH`, `DEV_GUILD_IDS` (guild-only command sync for testing), `TENOR_API_KEY`, `GENIUS_TOKEN`, `MAGICK_PATH`, `FFMPEG_PATH` (default `ffmpeg`), `RSS_INTERVAL` (default 5m) and `LOG_LEVEL`. Empty values SHALL behave as unset; invalid values MUST stop startup with a clear error.

#### Scenario: Missing token
- **WHEN** the bot starts without `DISCORD_TOKEN`
- **THEN** it exits saying DISCORD_TOKEN or DISCORD_TOKEN_FILE must be set

### Requirement: Nix build
The project SHALL be built with a flake-parts flake (never flake-utils) providing `packages.default` (static binary, CGO disabled, whole test suite run in the check phase), `packages.image` (OCI image with the bot, ImageMagick with liblqr, ffmpeg, CA certificates and tzdata) and a devShell with Go, gopls, golangci-lint, ImageMagick, ffmpeg, sqlite and openspec.

#### Scenario: Build the image
- **WHEN** a developer runs `nix build .#image`
- **THEN** an OCI archive is produced that `docker load` or `podman load` accepts
