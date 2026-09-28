# Mayuri2000

A Discord bot built on [disgo](https://github.com/disgoorg/disgo), replacing a
Red-DiscordBot instance. Every command is a slash command and can also be run
with a text prefix (default `%`, overridable per server with `/prefix`).

## Features

- **Images** (port of the NotSoBot cog): `magik`, `gmagik`, `caption`,
  `watermark`, `triggered`, `petimg`, `jpeg`, `glitch`, `pixelate`, `vw`,
  `waaw`/`haah`/`woow`/`hooh`, `flipimg`, `flop`, `invert`, `rotate`, `merge`,
  `iascii`, `ascii`, `rip`, `minecraftachievement`. Context menus: right-click a
  message → *Apps → Magik / JPEG / Glitch / Vaporwave / Invert*, or a user →
  *Triggered / Pet*.
- **Text**: `aesthetics`, `emojify`.
- **Moderation**: `kick`, `ban`, `hackban` (ban by ID, even users who are not in
  the server), `unban`, `timeout`, `untimeout`, `purge`.
- **Search**: `urban`, `wikipedia`, `youtube`, `ytsearch`, `lyrics` (Genius).
- **Info**: `serverinfo`, `stats` (uptime, commands run this session and all
  time, top commands, memory, latency).
- **Core**: `help`, `ping`, `prefix`, `alias`.

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
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

In the [Developer Portal](https://discord.com/developers/applications), enable
the **Message Content** privileged intent (needed for prefix commands).

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
DISCORD_TOKEN=... DEV_GUILD_IDS=<test guild> go run ./cmd/mayuri
```

## Building and deploying

```sh
nix build            # the static binary
nix build .#image    # OCI image archive (bot + ImageMagick + CA certs)
podman load < result # or: docker load < result
```

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
