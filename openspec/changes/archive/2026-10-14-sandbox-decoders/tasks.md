# Tasks

## 1. Sandbox package

- [x] 1.1 Add `github.com/landlock-lsm/go-landlock` and `github.com/elastic/go-seccomp-bpf`, update `vendorHash`; verify `nix build` succeeds with CGO still disabled
- [x] 1.2 Create `internal/sandbox` with the policy type (job dir, extra read-only paths), `Command(ctx, policy, path, args)` building an `exec.Cmd` that re-execs the bot binary with the `__mayuri-sandbox` marker and an explicit minimal `Env` (HOME/TMPDIR/MAGICK_TEMPORARY_PATH=job dir, MAGICK_CONFIGURE_PATH, LC_ALL=C), and `Main()` that, when the marker is present, applies rlimits, Landlock (ABI ≥1 required, newer ABIs best effort) and the seccomp denylist from design.md, then `syscall.Exec`s the target; verify with unit tests for policy encoding/decoding and that `Command` never inherits the parent environment
- [x] 1.3 Add `Probe()` (check mode reporting Landlock ABI, seccomp and rlimit status) and the `required`/`best-effort` decision; verify with a test that reports the parts applied on this machine
- [x] 1.4 Behaviour tests using the test binary as a fake decoder (via `TestMain` → `sandbox.Main()` and a test-only target mode): inside the sandbox, opening a file outside the job dir fails with EACCES, writing in the job dir works, `socket()` fails, `ptrace`/`unshare` fail, the environment contains only the minimal variables even when the parent has `DISCORD_TOKEN` set, and two jobs can't read each other's directories; skip with a message when Landlock is unavailable

## 2. Decoders through the sandbox

- [x] 2.1 Route `Magick.Run`, the magick `-version` probe, `FFmpeg.run`, `probe` and the ffmpeg `-version` probe through `sandbox.Command`; give each `Magick.Run` call its own job directory and write the caption font and watermark file into it (removing the shared font temp file); verify the full `go test ./...` effect suite passes sandboxed
- [x] 2.2 Trace decoder file access (`strace -f -e trace=%file` via `nix shell nixpkgs#strace`) over the imaging/notsobot effect tests and the video tests, and tighten or extend the Landlock read paths to exactly what is needed; verify the suite still passes and record the final path list in a comment
- [x] 2.3 Add `sandbox.Main()` to `TestMain` in `internal/imaging`, `internal/modules/notsobot` and `cmd/mayuri`; verify `go test ./...` passes from a clean cache

## 3. ImageMagick policy

- [x] 3.1 Embed a `policy.xml` (deny-all coders then allow the formats and pseudo-formats the effects use, deny `@*` paths, URL/delegate/MSL/MVG/SVG/TEXT coders, matching resource limits), write it to a read-only directory at startup and set `MAGICK_CONFIGURE_PATH`; verify `magick -list policy` inside the sandbox shows it, an SVG and `@file` input are refused in a test, and the effect suite passes

## 4. Startup and configuration

- [x] 4.1 Add `DECODER_SANDBOX` (`required` default, `best-effort`; anything else is a startup error) to `internal/config`; in `main`, run `sandbox.Probe()` before the magick/ffmpeg probes and in required mode mark both unavailable with a warning when it fails; verify with config tests (invalid value message) and a test of the decision logic
- [x] 4.2 Build the image and run it under the hardened container flags from `harden-container` with a fake token; verify the startup log reports the sandbox active (Landlock ABI, seccomp) and no "unavailable" warnings, then run magick and ffmpeg conversions through the bot's sandbox path inside that container

## 5. Docs and checks

- [x] 5.1 Document the decoder sandbox and `DECODER_SANDBOX` in the README (security/configuration) and `env.example`, and add the launcher/re-exec, job-directory and `TestMain` rules to CLAUDE.md; verify by reading
- [x] 5.2 Run `golangci-lint run ./...` (0 issues), `go test ./...`, `openspec validate --specs --strict` and `nix flake check`; verify all pass

## 6. Manual

- [x] 6.1 Deploy and run `/magik`, `/gmagik` on an MP4, `/caption` and `/watermark` in Discord; verify they work and the startup log shows the sandbox active
