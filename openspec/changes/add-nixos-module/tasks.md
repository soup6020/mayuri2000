# Tasks

## 1. Flake and module

- [x] 1.1 Add the `quadlet-nix` input and export `flake.nixosModules.default` via `importApply ./nix/module.nix { inherit withSystem; }`; verify `nix flake show` lists `nixosModules.default` and `nix flake check` still evaluates
- [x] 1.2 Write `nix/module.nix` with the options and generated config from design.md (user/group 10001, tmpfiles state dir, quadlet container with hardening, secret file mounts + `*_FILE` env, assertions for quadlet-nix presence and secrets in `environment`); verify by evaluating a minimal NixOS config and reading the generated quadlet unit text (image path, volumes, hardening keys, no token value)
- [x] 1.3 Add `restrictNetwork` (default true) setting `IPAddressDeny`/`IPAddressAllow` on the container's service as in design.md; verify the generated service unit contains the filters

## 2. VM test

- [x] 2.1 Add `checks.nixos-module` (x86_64-linux) with `runNixOSTest`: node imports quadlet-nix and the module with a fake token file; assert unit active, journal shows `decoder sandbox active`, `mayuri.db` owned by 10001 in the state dir, `podman inspect` shows user 10001, read-only root, CapDrop ALL, no-new-privileges, pids/memory limits, and the container env has `DISCORD_TOKEN_FILE` but not the token value; verify the test passes
- [x] 2.2 Extend the test with the network check: an HTTP server on the node's private address is refused from a probe running in the container's cgroup and network namespace, while the podman gateway DNS port is reachable; verify it passes, or (per design.md) drop `restrictNetwork` and report if the filter can't be made effective without breaking DNS
- [x] 2.3 Add an evaluation check that `environment.DISCORD_TOKEN` fails with the `tokenFile` hint; verify it is part of `nix flake check`

## 3. Docs and checks

- [x] 3.1 Replace the README quadlet snippet with a colmena + agenix + quadlet-nix example (inputs, imports, `age.secrets.mayuri-token` owned by `mayuri2000`, `services.mayuri2000`), document options, secret rotation (`systemctl restart mayuri2000`) and moving Compose data into `/var/lib/mayuri2000`; update CLAUDE.md (layout: `nix/`, commands: the VM test); verify by reading
- [x] 3.2 Run `golangci-lint run ./...`, `go test ./...`, `openspec validate --specs --strict` and `nix flake check` (including the VM test); verify all pass

## 4. Manual

- [ ] 4.1 Deploy to the colmena host with an agenix-managed token (after moving data if wanted); verify the bot comes online, image commands work, and `systemctl status mayuri2000` shows the hardened container
