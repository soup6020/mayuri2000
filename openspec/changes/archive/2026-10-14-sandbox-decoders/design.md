# Design

## Context

Decoders run in two places today.

- **ImageMagick:** `Magick.Run` (`internal/imaging/magick.go`) runs `exec.CommandContext(magick, -limit …, args)` with the image on stdin and the result on stdout. Helper files go to the shared `TMPDIR` and are passed by path:
  - the caption font, written once by `arialPath`;
  - the watermark image.
- **ffmpeg:** `internal/imaging/video.go` gives each job its own temp directory (`mayuri-video-*`), writes the input there, and runs `ffmpeg` (and a probe) on those files.
- No command sets `Env`, so children inherit the bot's whole environment.

Where the bot runs constrains the design:

- Inside the hardened container (`harden-container`): no capabilities, `no-new-privileges`, and Docker's default seccomp profile, which blocks `unshare` and namespace-creating `clone` for unprivileged processes. That was verified: `unshare -U` and `unshare -n` both fail with EPERM. So bubblewrap/nsjail-style sandboxes aren't an option.
- In development and in a future NixOS module: plain processes.
- On the deployment kernel (7.2): Landlock is active (`/sys/kernel/security/lsm` lists `landlock`).

See proposal.md for motivation.

## Goals / Non-Goals

**Goals:**
- A compromised decoder can't read secrets or the database, can't touch other jobs' files, and can't reach the network.
- Kernel attack surface from decoder processes is reduced.
- The sandbox works unchanged inside the hardened container, under podman, and on bare NixOS. It needs no privileges, so the bot keeps running as a normal user.
- The build stays CGO-free.

**Non-Goals:**
- Sandboxing the bot's own Go process. Its Go-parsed inputs (PNG/GIF/JPEG/WebP decoders in the standard library and x/image) are memory-safe. Its network access is needed and is limited to user URLs by `safehttp`.
- A strict seccomp allowlist for decoders. The reasons are under Decisions.
- Outbound filtering for the bot itself (systemd `IPAddressDeny`); that belongs in the NixOS-module work.

## Decisions

### Launcher: re-exec the bot binary

Decoder commands are built by `internal/sandbox` as `<bot executable> __mayuri-sandbox <encoded policy> -- <decoder> <args…>`.

- `sandbox.Main()` runs first in `main()`. When `os.Args[1]` is the marker, it never returns: it decodes the policy, applies the restrictions, then calls `syscall.Exec` on the decoder.
- Landlock domains, seccomp filters, `no_new_privs` and resource limits all carry across `execve`, so the decoder runs fully restricted from its first instruction.
- Packages whose tests run decoders (`internal/imaging`, `internal/modules/notsobot`, `cmd/mayuri`) call `sandbox.Main()` from `TestMain`, so the test binary can act as the launcher.
- The executable path is resolved once at startup (`os.Executable`).
- The marker and policy only ever come from our own code. User input reaches the decoder's arguments, exactly as it does today.

Alternatives considered:
- *bubblewrap:* blocked inside containers, as shown above.
- *A separate helper binary:* means one more package output, plus path configuration in the image and in tests, for no security gain.
- *Applying Landlock in the bot process itself:* it would restrict the bot too.
- *`SysProcAttr` hooks:* Go has no pre-exec callback in the forked child.

### Landlock filesystem rules

Uses `github.com/landlock-lsm/go-landlock` (pure Go; applies to all threads via `AllThreadsSyscall`, which needs CGO off and we have that).

- **Read and execute:** `/nix/store`. It holds the decoders, their libraries and ImageMagick's built-in config, and it never holds secrets; the "Secrets" requirement already forbids putting them there.
- **Read:**
  - the support directory (below);
  - `/sys/devices/system/cpu` (thread-count detection);
  - `/dev/null` (also writable).
- Tracing showed the decoders never open `/proc/cpuinfo`, `/proc/meminfo` or `/dev/urandom`, and they cope with `/proc/self`, `/proc/filesystems` and `/sys/devices/system/node` being denied. Those aren't granted.
- **Read and write:** only the job directory.
- The exact list is confirmed during implementation by tracing the decoders' file access (`strace -f -e trace=%file`) across the effect test suite. Anything they don't need stays denied.

Landlock ABI handling:
- ABI 1 (filesystem) is the minimum for "required" mode.
- Newer ABIs are used when available: refer and truncate rules, TCP bind/connect denial (ABI 4), and IPC scoping of abstract Unix sockets and signals (ABI 6). Missing newer ABIs only produce a debug log, because seccomp already blocks networking.

### Seccomp: allow by default, deny a fixed list

Uses `github.com/elastic/go-seccomp-bpf` (pure Go). The filter returns `EPERM` for:

- **Network:** `socket`, `socketpair`.
- **Process access:** `ptrace`, `process_vm_readv`/`writev`, `pidfd_getfd`.
- **Namespaces:** `unshare`, `setns`. `clone3` returns `ENOSYS`, so libc falls back to `clone`. `clone` with namespace flags is denied by an argument check.
- **Mounting:** `mount`, `umount2`, `pivot_root`, the new mount API (`fsopen`, `fsmount`, `move_mount`, `open_tree`).
- **Kernel interfaces:**
  - modules: `init_module`, `finit_module`, `delete_module`;
  - `kexec_*`, `bpf`, `perf_event_open`, `userfaultfd`, `io_uring_*`;
  - keyrings: `keyctl`, `add_key`, `request_key`;
  - `iopl`, `ioperm`, `swapon`/`swapoff`, `reboot`, `acct`, `settimeofday`, `clock_settime`.

Alternative considered: a traced allowlist. It would be stricter, but ImageMagick and ffmpeg take different syscall paths depending on codec and format. A path missed during tracing would break real user commands. This denylist removes the syscalls that matter for privilege escalation and data theft at no risk to correctness. It also matters outside containers (dev, NixOS), where Docker's profile doesn't apply.

### Resource limits (setrlimit before exec)

- `RLIMIT_CORE` 0.
- `RLIMIT_CPU` 150 s, matching ImageMagick's `-limit time 150`. A busy-looping exploit is killed even if it ignores signals from the context.
- `RLIMIT_FSIZE` 1 GiB.
- `RLIMIT_NOFILE` 256.
- Per-process memory is bounded by ImageMagick's own limits and, in the container, by the cgroup.

### Minimal environment

The launcher gets and passes on only:
- `HOME`, `TMPDIR` and `MAGICK_TEMPORARY_PATH`, all set to the job directory;
- `MAGICK_CONFIGURE_PATH` (the policy directory);
- `LC_ALL=C`.

`exec.Cmd.Env` is always set explicitly, so nothing is inherited. This removes the environment leak even in `best-effort` mode on kernels without Landlock.

### Per-job directories for ImageMagick

`Magick.Run` creates a `mayuri-magick-*` directory per call (in `TMPDIR`), makes it magick's working directory, and removes it afterwards. Per-call input files are written into it and passed by bare name through `RunWith`; that's the watermark image. A job can see only its own directory.

Static support files go in one read-only **support directory**, created once per process and registered with the sandbox as read-only:
- the caption font;
- the ImageMagick policy.

They are public assets, so sharing them leaks nothing. This replaces the shared font temp file without paying a 300 KB write per magick run; video effects run magick once per frame. (This changed during implementation: the original plan copied the font into every job directory.)

### ImageMagick policy

An embedded `policy.xml` is written once at startup to a read-only directory and pointed to by `MAGICK_CONFIGURE_PATH`. It:

- denies all coders, then allows the ones the bot uses: PNG, JPEG, GIF, WEBP, TTF, and pseudo-formats such as `xc`, `caption`, `null`, `gradient`, `plasma`. The exact list comes from grepping `magickfx.go` and running the effect tests.
- denies `path` `@*` (indirect reads);
- denies delegates and URL/`https`/`ephemeral`/`msl`/`mvg`/`svg`/`text`;
- sets resource limits matching the command-line `-limit` flags.

`magick -list policy` in a test confirms that ImageMagick loads it.

### Startup self-test and failure modes

At startup `sandbox.Probe()` launches the launcher in a check mode. It applies everything and reports which parts took effect: the Landlock ABI, seccomp, and rlimits.

- **Required mode (default):** if Landlock or seccomp failed, `magick` and `ffmpeg` are marked unavailable through the existing `Available` flags. The commands that need them reply with the existing "not available" message, and a startup warning says why.
- **`best-effort`:** whatever works is applied, and the missing parts are logged as warnings.
- The `-version` probes run through the sandbox too, so a sandbox that's too strict for the decoders shows up at startup rather than on the first user command.

## Risks / Trade-offs

- [A needed path is missed in the Landlock rules → effects fail with EACCES] → The full effect test suite (real magick and ffmpeg) runs sandboxed in `go test` and `nix flake check`, and file access is traced during implementation.
- [Docker or podman default seccomp profiles block `landlock_*` or `seccomp(2)` → the sandbox can't apply in containers] → Recent Docker and podman profiles allow both. Implementation verifies this in the hardened container. If they were blocked, required mode would disable effects loudly rather than silently run unsandboxed.
- [Per-frame magick runs on video pay one extra exec] → About 1–2 ms each, which is negligible next to magick's own startup.
- [Tests on machines without Landlock] → Sandbox tests skip with a message when the kernel reports no Landlock. Effect tests run in best-effort mode there, so they still pass on CI builders. The Nix build sandbox does support Landlock on recent kernels.
- [Re-exec marker in argv] → It's checked only as `os.Args[1]`, which no user input can set. The policy is serialized by our code and validated on decode.

## Migration Plan

No operator action is needed. The new image works under the hardened container settings. Operators on a kernel without Landlock (older than 5.13) either upgrade or set `DECODER_SANDBOX=best-effort`.
