# Design

## Context

`packages.image` is built with `dockerTools.buildLayeredImage`. Its contents are `cacert`, `tzdata`, a `/data` directory and a world-writable `/tmp`. There's no `User` and no `/etc/passwd`, so the bot runs as root. At runtime the bot writes only to:

- `DATABASE_PATH` (`/data/mayuri.db`, plus SQLite's `-wal`/`-shm` files);
- `TMPDIR` (`/tmp`), which ffmpeg's per-job temp directories and ImageMagick's pixel cache both use.

Because nothing else is written, a read-only root filesystem is safe. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:**
- No root inside the container, and a container that can't modify its own filesystem or gain privileges.
- Safe defaults in both documented deployments (Compose, quadlet), so copying the example gives the hardened setup.

**Non-Goals:**
- A custom seccomp allowlist for the whole container (see Decisions).
- Restricting outbound network access (LAN blocking). Compose and podman have no simple per-container egress filter. That belongs in the NixOS-module change, which can use systemd's `IPAddressDeny`.
- Scrubbing the environment passed to child processes, and sandboxing decoders. These are the next two changes.
- Rootless podman setup on the host. That's the operator's choice, and the image works either way.

## Decisions

- **Fixed UID/GID 10001, named `mayuri`.**
  - A numeric `User` means the runtime never has to resolve the name.
  - 10001 avoids clashing with common host users if someone runs rootful podman with bind mounts.
  - The passwd/group entries are still written so tools and libraries calling `getpwuid` behave. `HOME=/tmp` keeps anything that wants a home directory on the writable tmpfs.
  - Alternative considered: `nobody` (65534). It's shared with other services, which defeats per-service ownership of `/data`.
- **Ownership of `/data`** is set at image build time with `buildLayeredImage`'s `fakeRootCommands` (`chown 10001:10001 data`). Docker and podman copy the image directory's ownership into a *new* named volume, so fresh Compose installs need no extra step.
  - Bind mounts (the quadlet example) don't copy ownership. The README adds `systemd.tmpfiles.rules = [ "d /var/lib/mayuri 0700 10001 10001 -" ]`.
- **Seccomp: keep the runtime's default profile.**
  - It already blocks the dangerous syscalls (mount, kexec, bpf, keyctl, ptrace on older kernels, module loading, …).
  - A traced allowlist (e.g. from `oci-seccomp-bpf-hook`) would be fragile here. ffmpeg and ImageMagick take different syscall paths depending on format and codec, so an unseen path kills a job in production.
  - The examples only promise never to set `seccomp=unconfined`. A tight allowlist fits the planned decoder sandbox, whose jobs only read a file, compute and write a file.
- **Runtime settings** (Compose → quadlet):
  - `read_only: true` → `ReadOnly=true`. Quadlet's `ReadOnlyTmpfs` defaults to true and mounts tmpfs on `/tmp`, `/var/tmp` and `/run`.
  - `tmpfs: /tmp:size=1g,mode=1777` → an explicit `Tmpfs=/tmp:size=1g,mode=1777`.
  - `cap_drop: [ALL]` → `DropCapability=all`. The bot needs no capabilities: it binds no ports, changes no ownership, and makes only outbound connections.
  - `security_opt: [no-new-privileges:true]` → `NoNewPrivileges=true`.
  - `pids_limit: 512` → `PidsLimit=512`. The Go runtime's threads plus a few concurrent magick/ffmpeg processes stay far below it, while a fork bomb is stopped.
  - `mem_limit: 3g` → a memory limit. The cap of 3 GiB covers ImageMagick's own `-limit memory 512MiB` / `map 1GiB` per job, a few concurrent jobs, and the 1 GiB `/tmp` tmpfs (which counts as memory). If something runs away, the cgroup OOM killer usually picks the large magick/ffmpeg child, so the bot reports a failed command instead of dying.
  - The exact quadlet-nix attribute names are checked against its source while implementing, not guessed.
- **Clear error for an unwritable data directory:** before `store.Open`, `main` creates and removes a temp file in the database's directory. If that fails with a permission error, startup stops with: `database directory /data is not writable by uid 10001 (gid 10001); change its owner, e.g. chown -R 10001:10001 <dir>`. This turns the upgrade's breaking step into a self-explanatory message instead of a SQLite "readonly database" error at the first write.

## Risks / Trade-offs

- [Existing deployments break until their data is chowned] → The startup error explains the fix, and the README has an "Upgrading from a root image" note with commands for named volumes (a throwaway container that only runs `chown`) and bind mounts (`chown` on the host or a tmpfiles rule).
- [The memory limit could kill legitimate large jobs] → The value is a commented, tunable default. Jobs are already bounded by ImageMagick limits and video caps, and the limit rarely triggers.
- [A read-only root breaks a dependency that writes somewhere unexpected] → The task list includes running the real image under exactly these flags and exercising ImageMagick and ffmpeg (image and MP4 conversion) inside it.
- [Rootless podman maps UID 10001 to a subordinate UID on the host] → The bot doesn't care. The tmpfiles rule applies to rootful podman, and the README says so.

## Migration Plan

1. Build and load the new image.
2. Fix data ownership once:
   - Named volume: run a throwaway container that only runs `chown -R 10001:10001 /data`.
   - Bind mount: `chown -R 10001:10001 /var/lib/mayuri`, or apply the tmpfiles rule.
3. Restart.

Rollback: the old image runs as root and can still read files owned by 10001, so rolling back needs no ownership change.
