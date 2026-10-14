## MODIFIED Requirements

### Requirement: Configuration
The bot SHALL be configured only through environment variables: `DISCORD_TOKEN` (required), `BOT_PREFIX` (default `%`), `DATABASE_PATH`, `DEV_GUILD_IDS` (guild-only command sync for testing), `TENOR_API_KEY`, `GENIUS_TOKEN`, `MAGICK_PATH`, `FFMPEG_PATH` (default `ffmpeg`), `RSS_INTERVAL` (default 5m), `DECODER_SANDBOX` (`required` by default, or `best-effort`) and `LOG_LEVEL`. Empty values SHALL behave as unset; invalid values MUST stop startup with a clear error.

#### Scenario: Missing token
- **WHEN** the bot starts without `DISCORD_TOKEN`
- **THEN** it exits saying DISCORD_TOKEN or DISCORD_TOKEN_FILE must be set

#### Scenario: Invalid sandbox mode
- **WHEN** the bot starts with `DECODER_SANDBOX=off`
- **THEN** it exits saying DECODER_SANDBOX must be required or best-effort
