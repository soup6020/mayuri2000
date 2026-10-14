# Mayuri2000

Discord bot in Go on [disgo](https://github.com/disgoorg/disgo) (v0.19), replacing a
Red-DiscordBot instance/
User-facing docs are in `README.md`.

## Commands

```sh
nix develop -c go test ./...        # full suite (imaging tests use real `magick`, skip if absent)
nix develop -c golangci-lint run    # must report 0 issues; `golangci-lint fmt` fixes formatting
nix flake check                     # package build + tests, lint, and spec validation
nix build                           # static binary; checkPhase runs the whole test suite
nix build .#image                   # OCI image (bot + imagemagick w/ liblqr + ffmpeg + cacert), deployed via quadlet-nix
DISCORD_TOKEN=... DEV_GUILD_IDS=<test guild> nix develop -c go run ./cmd/mayuri
```

- Flakes only see git-tracked files: `git add` new files before `nix build`.
- The image runs as uid/gid 10001 and is meant for a read-only root filesystem: the bot may
  only write to `DATABASE_PATH`'s directory (`/data`) and `TMPDIR` (`/tmp`). `compose.yaml`
  and the README quadlet example carry the hardened runtime flags; keep them in sync.
- After changing `go.mod`/`go.sum`, update `vendorHash` in `flake.nix` (set it to
  `pkgs.lib.fakeHash`, build, copy the `got:` hash).
- Lint config is `.golangci.yml` (v2 format): standard linters plus bodyclose, errorlint,
  gocritic, noctx, etc.; gofmt/goimports with `mayuri2000` as the local import prefix.
  Fix findings rather than adding `//nolint`; `Close` on read-only resources and `os.Remove`
  are excluded from errcheck, but check `Close` errors on files you write.
- Store methods take a `context.Context` (noctx); pass the command `*cmd.Context`.
- Nix: use flake-parts (`perSystem`), never flake-utils.
- `IMAGING_OUT=<dir> go test ./internal/imaging/` writes every effect's output for visual checks.

## Specs (OpenSpec)

Intended behaviour lives in `openspec/specs/<capability>/spec.md` (requirements + scenarios);
this file covers code layout and workflow. Capabilities: `command-framework`, `image-commands`,
`moderation`, `search`, `rss-feeds`, `reminders`, `info-commands`, `utilities`, `safety`,
`deployment`. Project context and artifact rules are in `openspec/config.yaml`.

- Read the relevant spec before changing a feature: many behaviours there are deliberate
  decisions (e.g. RSS marks existing items seen, reminders only ping their owner,
  `swapcolors` keeps alpha, `nix shell nixpkgs#…` snippets) that the code alone doesn't explain.
- New features or behaviour changes go through a change proposal: `/opsx:propose "<idea>"`,
  then `/opsx:apply`, then `/opsx:archive` to merge its delta specs into `openspec/specs/`.
  Small fixes that don't change specified behaviour don't need a proposal.
- `openspec validate --specs --strict` must pass; `nix flake check` runs it (`checks.specs`).
  Requirements need SHALL/MUST, at most 500 characters, and at least one `#### Scenario:`.

## Layout

- `cmd/mayuri/` — main: config → store → magick probe → registry → `Validate()` → sync → gateway.
  Tests here build the full registry (`testRegistry`) and drive commands end to end.
- `internal/cmd/` — the command framework (see below).
- `internal/config/` — env config; secrets also accept `<NAME>_FILE`.
- `internal/store/` — SQLite (modernc, pure Go, no cgo). Tables are created with
  `CREATE TABLE IF NOT EXISTS` in `schema`; prefixes and aliases are cached in memory.
- `internal/tz/` — IANA zone lookup/search. `zones.go` is generated from Go's bundled tz
  database (`go generate ./internal/tz`, rerun after a Go upgrade); `time/tzdata` is embedded.
- `internal/sandbox/` — decoder sandbox. `sandbox.Command` re-execs the bot binary as a launcher
  (`sandbox.Main()`, first thing in `main()`) that applies rlimits, Landlock and a seccomp
  denylist, then execs ImageMagick/ffmpeg with a fixed minimal `Env`. Packages whose tests
  run decoders need `TestMain` → `sandbox.RunTests(m)` (see `sandbox_main_test.go`).
- `internal/safehttp/` — HTTP client for **user-supplied URLs**: refuses loopback/private/
  link-local/CGNAT addresses at dial time (SSRF guard). Always use it (or `safehttp.NewRequest`)
  when fetching a URL a user typed; fixed third-party APIs can use a normal client.
- `internal/htmlmd/` — HTML fragment → Discord markdown (option docs, feed summaries).
- `internal/rss/` — feed fetching (gofeed), seen-item tracking, templates/embeds, and the
  `Poller` started from `main` (every `RSS_INTERVAL`). `internal/modules/feeds/` holds `/rss`.
- `internal/units/` — unit conversion (`/convert`): units grouped by kind, each converting
  to/from the kind's base unit (kelvin, metre, metre/second); add a unit by adding a table
  entry (`linear(...)` for factor-only units). Display names use Canadian spelling.
- `internal/when/` — human time parsing: `ParseDuration` ("3h12m", also used by
  `/timeout`), `Parse` (delays, clock times, day words, ISO dates in a given location) and
  `Split` (greedy time prefix of a sentence, for prefix commands).
- `internal/modules/reminders/` — `/remind`, `/reminders`, and the `Scheduler` (started in
  `main`; sleeps until the next due reminder, `Wake()` after adding one). Delivery only
  pings the owner; DMs fall back to the source channel when closed; transient failures
  retry with backoff, permanent ones (unknown channel, missing access) drop the reminder.
- `internal/imagefind/` — locates a command's input (port of NotSoBot's ImageFinder). Returns
  `Source{URL, Kind}`: `Image`, `Video` (MP4) or `GIFVideo` (Tenor/Klipy MP4 — gifv embeds and
  tenor.com/klipy.com hosts). Klipy pages 403, so Klipy only arrives via Discord's gifv embed.
- `internal/imaging/` — decode/encode (GIF frames are fully composed), pure-Go effects, the
  ImageMagick CLI runner (`magick.go`, `magickfx.go`) and the ffmpeg video pipeline
  (`video.go`). A video decodes to an `Image` with `Video *VideoInfo` (fps, audio, GIFLike);
  `EncodeContext` then emits MP4 with the audio (or GIF for GIFLike). Effects must preserve
  `Video` when building a new `Image` and encode via `EncodeContext(ctx)`, not `Encode()`.
  ImageMagick effects take raw bytes, so on video they run per frame (`videoFrames` in notsobot).
- `internal/modules/{core,notsobot,moderation,search}/` — the commands.
- `assets/` — `go:embed`ded fonts, templates, petpet frames, vaporwave images.

## Command framework (`internal/cmd`)

Each `cmd.Command` is defined once and serves both slash and text-prefix invocations
(`Context` hides the transport). Rules:

- Register in `main.go` via `registry.Add(...)`. `Registry.Validate()` enforces Discord's
  limits at startup (names `^[a-z0-9_-]{1,32}$`, descriptions 1–100 chars, ≤25 options,
  ≤100 commands, ≤5 message/user context menus) and `TestCommandsValid` checks it in CI.
- Prefix args map to `Options` positionally in declaration order. Slash requires required
  options first; `slashCreate` reorders automatically.
  - `Rest: true` (String only) takes the remaining raw text — use for the last free-text arg.
  - `Accept` lets an optional option decline a token so it falls through to the next option
    (e.g. `url` only accepts image refs, `delete_days` only a single digit so user IDs never match).
  - `Channel` options (value: `snowflake.ID`, via `c.OptChannel`) take a mention, ID or
    `#name` for prefix use (`FindChannel`).
  - `Role` options take a mention, ID or name for prefix use (`FindRole`: exact, then unique
    prefix, then unique substring); `Rest` works for String and Role options.
  - `SlashOnly` options and `Attachment` options are never filled from prefix tokens;
    attachments come from the message's attachments. Unassigned tokens land in `c.Extras`.
- `Perms` sets slash default member permissions **and** is checked at runtime for both
  transports. `GuildOnly` restricts to servers.
- `Heavy` commands defer immediately, get a 3-minute context, and are serialised per guild
  plus bounded globally — use for anything doing image work or slow network calls.
- Return `cmd.Errorf(...)` for user-facing errors (shown ephemerally); any other error is
  logged and the user sees a generic message. Panics are recovered.
- Command groups: set `Subcommands` (slash `/rss add`, prefix `%rss add`). The group has no
  Options/Run; its `Perms`/`GuildOnly` apply to all subcommands (Discord only supports default
  permissions on the top-level command). Stats and logs use `FullName()` ("rss add").
- `Option.Autocomplete` (String options without `Choices`) supplies slash-command
  suggestions as the user types; results are capped at 25 and 100 chars each.
- `c.Paginate(pages)` for multi-page results; state lives in memory for 15 minutes and only
  the invoker can flip pages. Button custom IDs must be unique per message.
- Aliases are per-guild, prefix-only, resolved in `Registry.Resolve`; alias arguments come
  before the invocation's own.
- Stats: every run is recorded in `Registry.Stats()` (session) and `store.IncrementCommand` (all time).
- For tests, `cmd.NewTestContext(registry, args)` returns a context whose replies are captured;
  `cmd.RunPrefix(registry, command, "args text")` runs a command through the real text-argument
  binding (usage/invalid-argument replies included).

## Gotchas

- **Decoder sandbox**: never `exec.Command` magick/ffmpeg directly — go through
  `imaging.runSandboxed`/`Magick.RunWith`/`FFmpeg.command`. Each job gets its own temp dir (its
  only writable path, also its working directory); pass extra inputs as `RunWith` files and refer
  to them by bare name. Shared read-only files (policy, font) live in the support dir
  (`imaging/support.go`). New paths decoders need must be added deliberately in
  `sandbox/launcher.go` after tracing with strace; the ImageMagick policy
  (`imaging/policy/policy.xml`) only allows PNG/GIF/JPEG/WEBP/caption/null — `xc:` etc. are refused.
- **ImageMagick safety**: always pass input via `InputSpec` (explicit sniffed coder like
  `png:-`, never bare `-`) so untrusted data can't select MVG/SVG/MSL coders, and run any
  user text through `escapeText` (`@` reads files, `%` is a property escape).
- **ffmpeg safety**: untrusted video only via `DecodeVideo`: bytes are sniffed first (MP4 `ftyp`,
  not QuickTime), written to a per-job temp dir, and read with `-f mov -protocol_whitelist
  file,pipe` under the command context; never pass a URL or user string to ffmpeg. Video limits
  (10 s, 15 fps, 512 px longest side, never upscaled) are in `video.go`.
- Go's `image/jpeg` rejects corrupted data; `glitch` renders through magick (libjpeg is
  lenient) and falls back to Go decoding only when magick is unavailable.
- The embedded arial has no CJK/full-width glyphs (renders as boxes).
- Interaction replies: after a defer, the first reply edits the original response
  (`UpdateInteractionResponse`); don't send `embeds`/`components` as null there.
- RSS: subscribing stores all current item IDs as seen (no backlog spam); each poll posts at
  most 5 new items oldest-first; items that fail to post stay unseen and are retried (the
  ETag/Last-Modified cache is dropped so the next poll refetches). Feed posts use empty
  `AllowedMentions` so feed content can't ping anyone. Poller tests use an httptest server by
  swapping `Fetcher.client`, since the production client refuses loopback.
- `nixpkgs`/`nixoption` scrape search.nixos.org's frontend bundle for the Elasticsearch schema version,
  public credentials and channel list (cached 6h, falling back to `defaultNixosConfig`);
  bump the fallback if the site's schema version changes.
- Scraped sources (YouTube results, Genius, Tenor without a key) can break when those sites
  change; parsers are tested against saved responses in `internal/modules/search/testdata/`
  — refresh the fixtures from live responses when fixing one.
- Prefix commands need the privileged Message Content intent enabled in the Developer Portal.

## Conventions

- Match existing style: small focused files, comments explaining *why*, `slog` logging.
- Keep user-visible command descriptions short (≤100 chars) and aliases compatible with the
  old Red commands where they exist.
- Don't commit unless asked.
