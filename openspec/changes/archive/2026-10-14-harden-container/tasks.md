# Tasks

## 1. Image

- [x] 1.1 In `packages.image` add `etc/passwd` and `etc/group` entries for `mayuri` (10001:10001, home `/tmp`, shell `/sbin/nologin`) plus root, chown `/data` to 10001:10001 via `fakeRootCommands`, set `config.User = "10001:10001"` and `HOME=/tmp`; verify `nix build .#image` succeeds and `docker image inspect` shows `User` 10001:10001
- [x] 1.2 Load the image and run it with a throwaway token under `--read-only --tmpfs /tmp:size=1g,mode=1777 --cap-drop=ALL --security-opt no-new-privileges:true --pids-limit 512` on a fresh named volume; verify the logs show the database opened and the magick/ffmpeg probes succeeded before the expected Discord login failure, and that `mayuri.db` in the volume is owned by 10001
- [x] 1.3 Under the same flags, run `magick` (via `--entrypoint`) converting a PNG from stdin and resizing a GIF, and `ffmpeg` re-encoding a short generated MP4 into `/tmp`; verify both succeed (no read-only filesystem or permission errors)

## 2. Startup check

- [x] 2.1 Before `store.Open` in `cmd/mayuri/main.go`, check the database directory is writable (create and remove a temp file) and on a permission error fail with a message naming the directory, the current uid/gid and the `chown -R <uid>:<gid> <dir>` fix; verify with a unit test using a read-only temp directory (skipped when running as root)
- [x] 2.2 Run the image as in 1.2 against a root-owned volume; verify it exits with that message

## 3. Deployment examples and docs

- [x] 3.1 Update `compose.yaml` with `read_only`, `tmpfs` (/tmp, 1g, 1777), `cap_drop: [ALL]`, `security_opt: [no-new-privileges:true]`, `pids_limit: 512`, `mem_limit: 3g` (each commented); verify `docker compose config` accepts it and `./docker-up.sh`-style startup under it works as in 1.2
- [x] 3.2 Check quadlet-nix's option names in its source, then update the README quadlet example with the equivalent settings (ReadOnly, Tmpfs, DropCapability all, NoNewPrivileges, PidsLimit, memory limit) and a `systemd.tmpfiles.rules` entry owning `/var/lib/mayuri` by 10001; verify by reading the section and evaluating the snippet's attribute names against quadlet-nix
- [x] 3.3 Add an "Upgrading from a root image" README note with the one-time chown commands for a named volume and a bind mount, and mention the non-root user in the CLAUDE.md image line; verify by reading both
- [x] 3.4 Run `golangci-lint run ./...` (0 issues), `go test ./...` and `nix flake check`; verify all pass

## 4. Manual

- [x] 4.1 Deploy the hardened image with the real token (after chowning the existing data); verify the bot comes online with existing prefixes/feeds intact and that `/magik`, `/gmagik` on an MP4, and `/caption` work
