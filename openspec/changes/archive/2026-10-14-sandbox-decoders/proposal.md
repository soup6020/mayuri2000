# Proposal

## Why

ImageMagick and ffmpeg parse files uploaded by strangers. They are the bot's largest attack surface, and both have long CVE histories. Today a successful exploit would run with everything the bot has:

- **Every environment variable.** No `exec.Cmd` sets `Env`, so `DISCORD_TOKEN` (when passed as a variable), `GENIUS_TOKEN` and `TENOR_API_KEY` are inherited by every child process.
- **Every file the bot's user can read**, including a `DISCORD_TOKEN_FILE` secret and the SQLite database.
- **Full network access.**

The container hardening (`harden-container`) limits damage to the host but not to the bot's own secrets and data. Inside a hardened container, namespace sandboxes such as bubblewrap can't start: Docker's default seccomp profile blocks `unshare` without capabilities, which was confirmed by testing. So the sandbox has to use mechanisms that need no privileges.

## What Changes

- **Minimal environment for decoders.** ImageMagick and ffmpeg receive only a fixed set of variables, never the bot's environment:
  - `HOME`, `TMPDIR` and `MAGICK_TEMPORARY_PATH`, all pointing at the job directory;
  - `MAGICK_CONFIGURE_PATH`;
  - `LC_ALL=C`.
- **Per-job sandbox.** Each ImageMagick or ffmpeg run (including the startup `-version` probes) goes through a small launcher built into the bot binary. Before exec'ing the decoder, it applies:
  - **Landlock filesystem rules:**
    - read-only: `/nix/store` and a few `/proc`, `/sys` and `/dev` entries the decoders need;
    - read-write: only that job's own temporary directory;
    - nothing else (no `/data`, no secrets, no other jobs' files).
  - **No network:** a seccomp filter denies `socket`/`socketpair`, and Landlock also denies TCP connect/bind where the kernel supports it.
  - **A seccomp denylist** of syscalls a decoder never needs: tracing other processes, namespaces, mounting, kernel modules, BPF, io_uring, userfaultfd, perf events and keyrings.
  - **Resource limits:** CPU time, maximum file size, open files, and no core dumps.
- **Per-job directories.** Every ImageMagick run gets its own temporary directory. Helper files (the caption font, watermark images) are written into it instead of the shared temp directory.
- **ImageMagick security policy.** A `policy.xml` allows only the formats and pseudo-formats the bot uses, blocks indirect reads (`@file`) and URL/delegate coders, and repeats the resource limits. It's a second layer behind the forced input coder.
- **Fail closed by default.** If the sandbox can't be applied (the kernel lacks Landlock, or the seccomp filter is rejected), ImageMagick and ffmpeg features are disabled with a startup warning, the same way a missing ImageMagick is handled today. `DECODER_SANDBOX=best-effort` lets an operator run with whatever subset applies, and logs which parts are missing.
- **New dependencies:** pure-Go Landlock and seccomp-BPF libraries. The build stays CGO-free, and `vendorHash` changes.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `safety`:
  - "ImageMagick input handling" and "ffmpeg input handling" add the sandbox, the minimal environment and the ImageMagick policy.
  - New requirement "Decoder sandbox".
  - "Secrets" adds that secrets never reach child processes.
- `deployment`: "Configuration" adds `DECODER_SANDBOX`.

## Impact

- New `internal/sandbox` package (launcher, policy, re-exec entry point). `main()` and the `TestMain` of packages that run decoders call its entry point first.
- `internal/imaging/magick.go`, `magickfx.go`, `video.go`: commands are built through the sandbox; per-job directories; font and watermark files move into them.
- `cmd/mayuri/main.go`: sandbox self-test at startup that decides whether ImageMagick and ffmpeg are enabled.
- `internal/config`: `DECODER_SANDBOX`.
- An embedded ImageMagick `policy.xml`.
- `go.mod`/`go.sum`, and `vendorHash` in `flake.nix`.
- README (security section, configuration), CLAUDE.md (gotchas and layout).
- Performance: one extra exec per decoder run (~1–2 ms). Video and GIF effects run magick once per frame, so this adds about 0.1–0.3 s for a 150-frame video.
