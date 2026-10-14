# Proposal

## Why

The bot is public-facing, and its main attack surface is ImageMagick and ffmpeg parsing files that strangers upload. If one of those is exploited today, the attacker gets **root inside the container**: the image sets no `User`, and nothing in the Compose or quadlet examples limits capabilities, privilege escalation or writes to the image filesystem. This change contains a compromise as much as the container runtime allows. It's the first of three hardening steps. The later ones scrub the environment passed to child processes and sandbox each decoder run; they're planned as separate changes.

## What Changes

- **The image runs the bot as an unprivileged user**, `mayuri` (UID/GID 10001):
  - `/etc/passwd` and `/etc/group` entries are added.
  - `HOME=/tmp`.
  - `/data` is owned by 10001, so new named volumes start with the right ownership.
- **Hardened runtime settings in `compose.yaml`:**
  - read-only root filesystem;
  - tmpfs `/tmp` (size-capped);
  - all capabilities dropped and `no-new-privileges`;
  - the runtime's default seccomp profile, which stays enabled (never `unconfined`);
  - pids and memory limits.
- **The README quadlet example** gets the same settings: `ReadOnly`, `DropCapability=all`, `NoNewPrivileges`, pids and memory limits. It also gets a tmpfiles rule so the bind-mounted state directory is owned by UID 10001.
- **Clear startup error:** if the database directory isn't writable, the bot stops with a message that names the UID it runs as and how to fix the ownership. Without this, it would fail with an obscure SQLite error.
- **BREAKING (deployment only):** existing volumes or bind mounts created by the old root-run image hold root-owned files. They need a one-time `chown -R 10001:10001`, and the README documents the command.
- No new seccomp profile is shipped. The reasoning is in design.md; a tight profile belongs to the later decoder-sandbox change.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `deployment`:
  - New requirement: the container runs as a non-root user, and the documented deployments apply the hardened runtime settings.
  - New requirement: the clear error for an unwritable data directory.
  - "Nix build": records that the image runs as a non-root user.

## Impact

- `flake.nix` (`packages.image`): user entries, `/data` ownership, `User`, `HOME`.
- `cmd/mayuri/main.go`: a data-directory writability check before opening the database, plus a test.
- `compose.yaml`, the README (Compose and quadlet sections, upgrade note), `env.example` (unchanged values, comment only if needed).
- Operators: a one-time ownership migration for existing data.
- No Go dependency changes; `vendorHash` is unaffected.
