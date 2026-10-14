# Proposal

## Why

The bot will run on a NixOS host that is already deployed with colmena, with secrets managed by agenix. Today the only quadlet-nix guidance is a README snippet. Every host would copy those hardening flags, the tmpfiles rule and the secret handling by hand, and could drift from the image's requirements: UID 10001, read-only root, `/tmp` tmpfs. A NixOS module exported by this flake makes the hardened setup the default. A colmena deploy that bumps the flake input then rolls out the new image and restarts the bot automatically.

## What Changes

- **New `nixosModules.default`** exported by the flake: `services.mayuri2000`. It runs the OCI image through quadlet-nix as a rootful podman system container, with the same hardening as the README/Compose examples:
  - read-only root and size-capped `/tmp`;
  - all capabilities dropped and `no-new-privileges`;
  - default seccomp;
  - pids and memory limits;
  - the decoder sandbox (`DECODER_SANDBOX=required`).
- **Secrets as files only:**
  - `tokenFile` (required), plus optional `geniusTokenFile` and `tenorApiKeyFile`, are bind-mounted read-only into the container and passed as `*_FILE` variables. The token never appears in an environment variable, a unit file or the Nix store.
  - Designed for agenix (`config.age.secrets.<name>.path`), but any file works.
  - Secret values in `environment` are rejected at evaluation time.
- **Host user and state directory:**
  - The module creates a `mayuri2000` system user and group with UID/GID 10001, matching the image, so agenix secrets can be owned by it.
  - It creates `dataDir` (default `/var/lib/mayuri2000`, mode 0700) owned by that user.
- **Restart on update:** quadlet references the image by its Nix store path, so a new image changes the unit and `colmena apply` restarts the bot. No registry and no `latest` tag are involved.
- **Outbound network restriction** (`restrictNetwork`, default on): systemd's per-unit IP filtering denies connections from the container to loopback, private, link-local and CGNAT ranges, except podman's DNS. The bot can't be used to reach the host's LAN even if `safehttp` were bypassed.
- **README:** a colmena + agenix + quadlet-nix example replaces the hand-written quadlet snippet. The Compose example stays.
- **Testing:** a NixOS VM test (`checks.nixos-module`, x86_64-linux) boots a node with the module and checks:
  - the user, ownership and hardening;
  - that the token stays out of the environment;
  - the decoder sandbox;
  - the network restriction.
- A flake input `quadlet-nix` is added. It's used by the VM test, and hosts can `follows` it.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `deployment`:
  - "Deployment methods" makes the NixOS module the primary deployment, with Compose secondary.
  - New requirements: "NixOS module", "Secret files in the NixOS module", "Network restriction".
  - "Hardened deployment examples" now covers the module rather than a README snippet.

## Impact

- `flake.nix`: the `quadlet-nix` input, `flake.nixosModules.default` (via flake-parts `withSystem` for the default image), and `checks.nixos-module`.
- New `nix/module.nix` and `nix/test.nix`.
- README deployment section; CLAUDE.md layout and commands.
- `nix flake check` gets slower by one VM test (about a minute or two on KVM). It runs only on x86_64-linux.
- No Go code changes.
