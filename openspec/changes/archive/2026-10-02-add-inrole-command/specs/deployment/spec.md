# Spec Delta

## ADDED Requirements

### Requirement: Privileged intents
The Discord application SHALL have the Message Content privileged intent enabled (for text-prefix commands) and the Server Members privileged intent enabled (for listing role members). The README MUST document both as setup steps.

#### Scenario: Fresh setup
- **WHEN** someone follows the README to set up the bot
- **THEN** they are told to enable both the Message Content and Server Members intents in the Developer Portal

## MODIFIED Requirements

### Requirement: Nix build
The project SHALL be built with a flake-parts flake (never flake-utils) providing `packages.default` (static binary, CGO disabled, whole test suite run in the check phase), `packages.image` (OCI image with the bot, ImageMagick with liblqr, CA certificates and tzdata) and a devShell with Go, gopls, golangci-lint, ImageMagick, sqlite and openspec.

#### Scenario: Build the image
- **WHEN** a developer runs `nix build .#image`
- **THEN** an OCI archive is produced that `docker load` or `podman load` accepts

### Requirement: Quality gates
`nix flake check` SHALL build and test the package, run golangci-lint using `.golangci.yml`, and validate the OpenSpec specs strictly; changes MUST pass it with 0 lint issues. Third-party API and scraped-page parsers SHALL be tested against saved fixtures, and ImageMagick tests SHALL skip when magick is unavailable.

#### Scenario: Lint failure
- **WHEN** a change introduces an unchecked error
- **THEN** `nix flake check` fails

#### Scenario: Malformed spec
- **WHEN** a spec requirement has no scenario
- **THEN** `nix flake check` fails
