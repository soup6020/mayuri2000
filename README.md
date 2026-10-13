# Mayuri2000

A Discord bot built on [disgo](https://github.com/disgoorg/disgo), replacing a
Red-DiscordBot instance. Every command is a slash command and can also be run
with a text prefix (default `%`, overridable per server with `/prefix`).

## Features

- **Images** (port of the NotSoBot cog): `magik`, `gmagik`, `caption`,
  `watermark`, `triggered`, `petimg`, `jpeg`, `glitch`, `pixelate`, `vw`,
  `waaw`/`haah`/`woow`/`hooh`, `flipimg`, `flop`, `invert`, `rotate`, `merge`,
  `iascii`, `ascii`, `rip`, `minecraftachievement`, plus effects from upstream
  NotSoBot: `deepfry`, `solarize`, `shake`, `swapcolors`, `swappixels`,
  `swaprgba`, `wall`, `watercolor`. Context menus: right-click a
  message → *Apps → Magik / JPEG / Glitch / GMagik / Invert*, or a user →
  *Triggered / Pet*.
- **Text**: `aesthetics`, `emojify`.
- **Moderation**: `kick`, `ban`, `hackban` (ban by ID, even users who are not in
  the server), `unban`, `timeout`, `untimeout`, `purge`.
- **Search**: `urban`, `wikipedia`, `wikifeatured [date]` (featured article, picture of
  the day, most read), `wikirandom` (with a 🎲 reroll button), `youtube`, `ytsearch`,
  `lyrics` (Genius), `scp <number>` (SCP Foundation article name, rating and link),
  `nixpkgs` and `nixoption` (search.nixos.org packages and NixOS options;
  stable or unstable channel).
- **Info**: `userinfo [user]` (also *Apps → User Info* on a user), `roleinfo
  <role>` (mention, ID or name), `inrole <role>` (list the members who have a role),
  `serverinfo`, `stats` (uptime, commands run this session and all
  time, top commands, memory, latency), `time [zone]` (any IANA tz name, e.g.
  `Europe/London`; case-insensitive, autocompleted for slash commands).
- **Feeds**: `rss add|remove|list|template|embed|force|find` — subscribe channels to
  RSS/Atom/JSON feeds; new items are posted automatically (needs Manage Channels).
- **Reminders**: `remind <when> <message> [channel]` (DM by default, or post in a
  channel), `reminders list|cancel`, and `timezone` to set your own time zone.
- **Fun**: `frogtip` (a random tip from frog.tips).
- **Tools**: `base64 encode|decode` (text or a file; standard or URL-safe),
  `convert <from> <to> <value>` (temperature: °C, °F, K; distance: km, mi, m, ft;
  speed: km/h, mph; e.g. `/convert from:F to:C value:100`, `%convert c f 30` or
  `%convert km mi 10`).
- **Core**: `help`, `ping`, `prefix`, `alias`.

### Reminders

```
/remind when:3h12m message:take out the laundry          # DMs you
/remind when:20:15 message:standup channel:#team         # posts in #team and pings you
%remind tomorrow 9am call mom                            # prefix form
%remind #team friday 18:00 pub!                          # prefix, channel first
```

`when` accepts delays (`3h12m`, `90m`, `in 2 hours`), clock times (`20:15`, `8:15pm`,
`noon`), day words (`tomorrow 9am`, `friday 18:00`) and dates (`2026-10-01 20:15`), up
to 21 days ahead. Clock times use your `/timezone` (UTC until you set one). Reminders
are stored in the database, so they survive restarts; ones that came due while the
bot was down are delivered late with a note. If your DMs are closed, the reminder
is posted in the channel where you set it.

### RSS feeds

```
/rss find url:bonequest.com                 # discover a site's feed URLs
/rss add name:jerj url:https://www.bonequest.com/bonequest.rss channel:#comics
/rss template name:jerj template:**$title**\n$link
/rss embed name:jerj enabled:true           # rich embeds instead of the template
/rss force name:jerj                        # post the latest item now
```

Feeds are checked every `RSS_INTERVAL` (default 5m). Subscribing marks existing
items as seen, so only new items are posted (at most 5 per check). Templates use
`$title`, `$link`, `$summary`, `$author`, `$published`, `$categories`, `$image`,
`$feed` and `$name`; Red's names like `$summary_plaintext` and `$content_image01`
still work.

### Aliases

Servers can define aliases for text-prefix commands (they do not create slash
commands). An alias runs a command with optional leading arguments; anything
typed after the alias is appended:

```
%alias add mg magik 5      # needs Manage Server
%mg https://example.com/cat.png   → %magik 5 https://example.com/cat.png
%alias list
%alias remove mg
```

Image commands find their input in this order: the `image` attachment, the
`url` argument (image link, Tenor link, custom emoji, user mention or ID), the
`user` option, the replied-to message, then the last 10 messages in the
channel. With a prefix, e.g. `%magik`, `%magik @someone`, `%rotate 45 <url>`,
or reply to an image with `%jpeg`.

**Video:** image effects also accept MP4 (uploads, `.mp4` links and video
embeds). An MP4 comes back as an MP4 with every frame processed and the
original audio kept. Tenor and Klipy GIFs (which Discord serves as MP4) are
read from their MP4 and come back as looping GIFs; run a command right after
someone posts one, reply to it, or use the right-click menu. Videos are
limited to the first 10 seconds, 15 fps and 512 px on the longest side.
Effects that work on a single frame (jpeg, glitch, vw, petimg, triggered,
merge) use the first frame. MOV and WebM aren't supported.

## Configuration

All configuration is through environment variables. `_FILE` variants read the
value from a file (useful with podman secrets).

| Variable | Default | |
|---|---|---|
| `DISCORD_TOKEN` / `DISCORD_TOKEN_FILE` | – | Bot token (required) |
| `BOT_PREFIX` | `%` | Default text prefix |
| `DATABASE_PATH` | `mayuri.db` (`/data/mayuri.db` in the image) | SQLite database for per-server settings |
| `DEV_GUILD_IDS` | – | Comma-separated guild IDs. When set, commands are synced to these guilds only (instant); otherwise globally |
| `TENOR_API_KEY` / `TENOR_API_KEY_FILE` | – | Optional. Without it, Tenor links are resolved by scraping |
| `GENIUS_TOKEN` / `GENIUS_TOKEN_FILE` | – | Optional Genius API client access token for song search; without it the public search endpoint is used |
| `MAGICK_PATH` | `magick` | ImageMagick 7 binary (needs liblqr for magik) |
| `FFMPEG_PATH` | `ffmpeg` | ffmpeg binary for MP4 video input; without it, images still work |
| `RSS_INTERVAL` | `5m` | How often feeds are checked (minimum `1m`) |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

In the [Developer Portal](https://discord.com/developers/applications), under
Bot → Privileged Gateway Intents, enable:

- **Message Content** (needed for prefix commands)
- **Server Members** (needed by `/inrole` to list a role's members)

Invite with the `bot` and `applications.commands` scopes and these permissions
(View Channels, Send Messages, Embed Links, Attach Files, Read Message History,
Manage Messages, Kick, Ban, Moderate Members):

```
https://discord.com/oauth2/authorize?client_id=<APP_ID>&scope=bot+applications.commands&permissions=1099511753734
```

## Development

```sh
nix develop            # go, gopls, golangci-lint, imagemagick (with liblqr)
go test ./...
golangci-lint run      # config in .golangci.yml; `golangci-lint fmt` to format
nix flake check        # builds + tests the package and runs the linters
DISCORD_TOKEN=... DEV_GUILD_IDS=<test guild> go run ./cmd/mayuri
```

## Building and deploying

```sh
nix build            # the static binary
nix build .#image    # OCI image archive (bot + ImageMagick + CA certs)
podman load < result # or: docker load < result
```

### Docker Compose

`compose.yaml` is a minimal example that reads its configuration from `.env`:

```sh
cp env.example .env    # then set DISCORD_TOKEN
./docker-up.sh         # builds and loads the image with Nix if it's missing, then `docker compose up -d`
docker compose logs -f
```

`docker-up.sh` only builds the image when `mayuri2000:latest` isn't present; after
updating the bot, rebuild it yourself with `nix build .#image && docker load < result`
(then `./docker-up.sh` recreates the container). Extra arguments are passed to
`docker compose up`.

The database is kept in the `mayuri-data` volume.

With [quadlet-nix](https://github.com/SEIAROTg/quadlet-nix), something like:

```nix
virtualisation.quadlet.containers.mayuri = {
  autoStart = true;
  containerConfig = {
    image = "docker-archive:${inputs.mayuri2000.packages.${pkgs.system}.image}";
    volumes = [ "/var/lib/mayuri:/data" ];
    # File containing DISCORD_TOKEN=... (keep it out of the Nix store)
    environmentFiles = [ "/run/secrets/mayuri.env" ];
    environments.BOT_PREFIX = "%";
  };
};
```

The container keeps its state in `/data`.
