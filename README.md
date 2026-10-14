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
| `DECODER_SANDBOX` | `required` | `required`: ImageMagick/ffmpeg only run fully sandboxed, otherwise image and video effects are disabled. `best-effort`: run with whatever parts of the sandbox the kernel supports |

### Decoder sandbox

ImageMagick and ffmpeg parse files strangers upload, so every run is
sandboxed (no privileges or namespaces needed, so it works inside the
hardened container too):

- **Environment**: only `HOME`/`TMPDIR` (the job's own directory),
  `MAGICK_CONFIGURE_PATH` and `LC_ALL` — never the bot's token or API keys.
- **Files** (Landlock, Linux 5.13+): read-only access to `/nix/store` and a
  read-only support directory (ImageMagick policy, caption font); read-write
  access only to the job's own temporary directory. The database, secret files
  and other jobs' files are off limits.
- **Syscalls** (seccomp): no network sockets, ptrace, namespaces, mounts,
  kernel modules, BPF, io_uring, userfaultfd, perf events or keyrings.
- **Limits**: CPU time, output file size and open files; no core dumps.
- **ImageMagick policy**: only PNG, GIF, JPEG, WebP, `caption:` and `null:`;
  no delegates and no `@file` reads.

The startup log says `decoder sandbox active` with the Landlock ABI version.
On kernels without Landlock the effects are disabled unless
`DECODER_SANDBOX=best-effort` is set.

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
nix build .#image    # OCI image archive (bot + ImageMagick + ffmpeg + CA certs)
podman load < result # or: docker load < result
```

### NixOS module (colmena, agenix, quadlet-nix)

The flake exports `nixosModules.default`, which runs the image as a hardened
rootful podman container through [quadlet-nix](https://github.com/SEIAROTg/quadlet-nix).
Your host flake (e.g. a colmena hive) imports it alongside quadlet-nix and,
for secrets, agenix:

```nix
# flake.nix inputs
mayuri2000 = {
  url = "git+ssh://…/Mayuri2000";
  inputs.nixpkgs.follows = "nixpkgs";       # optional: build against your nixpkgs
  inputs.quadlet-nix.follows = "quadlet-nix"; # only used by Mayuri2000's own tests
};
```

```nix
# the host's configuration
{ config, inputs, ... }:
{
  imports = [
    inputs.agenix.nixosModules.default
    inputs.quadlet-nix.nixosModules.quadlet
    inputs.mayuri2000.nixosModules.default
  ];

  # The file holds only the token. The module creates the mayuri2000 user
  # (uid/gid 10001, the uid the bot runs as), which must be able to read it.
  age.secrets.mayuri-token = {
    file = ./secrets/mayuri-token.age;
    owner = "mayuri2000";
    group = "mayuri2000";
  };

  services.mayuri2000 = {
    enable = true;
    tokenFile = config.age.secrets.mayuri-token.path;
    environment.BOT_PREFIX = "%";
  };
}
```

| Option | Default | |
|---|---|---|
| `tokenFile` | – | File containing only the bot token (required) |
| `geniusTokenFile`, `tenorApiKeyFile` | `null` | Optional API key files |
| `environment` | `{ }` | Non-secret settings (`BOT_PREFIX`, `DEV_GUILD_IDS`, `LOG_LEVEL`, `RSS_INTERVAL`, `DECODER_SANDBOX`); secrets here fail evaluation |
| `dataDir` | `/var/lib/mayuri2000` | Database directory (mode 0700, owned by `mayuri2000`) |
| `memory`, `pidsLimit`, `tmpSize` | `3g`, `512`, `1g` | Container limits |
| `restrictNetwork` | `true` | Block connections to loopback, private, link-local, CGNAT and multicast addresses (your LAN) |
| `subnet`, `gateway` | `10.89.173.0/24`, `.1` | The container's dedicated podman network; its gateway is the DNS server |
| `image` | this flake's image | OCI image archive to run |

What it sets up:

- **Secrets as files only**: each `*File` is mounted read-only at
  `/run/secrets/…` in the container and passed as `DISCORD_TOKEN_FILE` etc.
  The token is never in an environment variable, unit file or the Nix store.
- **Hardening**: the same as `compose.yaml` (non-root uid 10001, read-only root,
  size-capped `/tmp`, no capabilities, `no-new-privileges`, default seccomp,
  limits) plus the decoder sandbox.
- **No LAN access** (`restrictNetwork`): systemd's IP filtering on the unit
  drops the container's packets to private and local addresses (connections
  time out); only the container network's DNS server on the gateway is
  allowed. Public addresses, including Discord, work normally.
- **Updates**: the unit refers to the image by its Nix store path, so after
  `nix flake update mayuri2000`, `colmena apply` restarts the bot only when the
  image changed.

Day to day:

```sh
agenix -e secrets/mayuri-token.age          # rotate the token, deploy, then:
ssh host systemctl restart mayuri2000       # the mounted secret is only re-read on restart
journalctl -u mayuri2000 -f                 # logs (look for "decoder sandbox active")
```

Moving data from Docker Compose: stop Compose, then copy the volume's database
into `dataDir` before the bot first starts there (or stop it with
`systemctl stop mayuri2000`):

```sh
docker compose down
docker run --rm -v mayuri2000_mayuri-data:/data:ro busybox tar -C /data -cf - . \
  | ssh host 'sudo tar -C /var/lib/mayuri2000 -xf - && sudo chown -R 10001:10001 /var/lib/mayuri2000'
```

`nix flake check` runs the module in a NixOS VM (`checks.nixos-module`,
x86_64-linux with KVM): user and ownership, hardening, the token staying out of
the environment, the decoder sandbox, DNS, and that the LAN is unreachable.

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

The container is hardened: it runs as an unprivileged user (uid/gid 10001)
with a read-only root filesystem, a size-capped `/tmp`, no capabilities,
`no-new-privileges`, the default seccomp profile, and process and memory limits.


### Upgrading from a root image

Images built before the container hardening ran as root, so existing data is
owned by root and the bot (now uid 10001) can't write to it. It stops at startup
with `database directory /data is not writable by uid 10001`. Change the owner once:

```sh
# Docker Compose named volume (the name is <project>_mayuri-data)
docker compose down
docker run --rm -v mayuri2000_mayuri-data:/data busybox chown -R 10001:10001 /data

# NixOS module data directory
sudo chown -R 10001:10001 /var/lib/mayuri2000

# rootless podman bind mount
podman unshare chown -R 10001:10001 /path/to/data
```

Rolling back to an older image needs no change: root can read the files.
