# Evaluation-only check: secrets in services.mayuri2000.environment must fail
# with a hint to use the *File options (checks.nixos-module-rejects-env-secrets).
{
  pkgs,
  lib,
  nixpkgs,
  system,
  quadletModule,
  mayuriModule,
}:
let
  host = nixpkgs.lib.nixosSystem {
    inherit system;
    modules = [
      quadletModule
      mayuriModule
      {
        boot.isContainer = true;
        system.stateVersion = "25.05";
        services.mayuri2000 = {
          enable = true;
          tokenFile = "/run/agenix/mayuri-token";
          environment.DISCORD_TOKEN = "leaked";
        };
      }
    ];
  };
  failed = map (a: a.message) (lib.filter (a: !a.assertion) host.config.assertions);
  rejected = lib.any (m: lib.hasInfix "DISCORD_TOKEN" m && lib.hasInfix "use tokenFile" m) failed;
in
if rejected then
  pkgs.runCommand "mayuri2000-module-rejects-env-secrets" { } "touch $out"
else
  throw "services.mayuri2000 accepted DISCORD_TOKEN in environment (failed assertions: ${builtins.toJSON failed})"
