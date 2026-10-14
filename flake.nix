{
  description = "Mayuri2000 - a Discord bot built on disgo";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
    # Only used by the NixOS module's VM test; hosts bring their own.
    quadlet-nix.url = "github:SEIAROTg/quadlet-nix";
  };

  outputs =
    inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } (
      { withSystem, flake-parts-lib, ... }:
      {
        # services.mayuri2000: the hardened container via quadlet-nix.
        flake.nixosModules.default = flake-parts-lib.importApply ./nix/module.nix { inherit withSystem; };

        systems = [
          "x86_64-linux"
          "aarch64-linux"
        ];

        perSystem =
          {
            pkgs,
            self',
            system,
            lib,
            ...
          }:
          let
            magick = pkgs.imagemagick.override { liblqr1Support = true; };
            ffmpeg = pkgs.ffmpeg-headless;
          in
          {
            packages.default = pkgs.buildGoModule {
              pname = "mayuri2000";
              version = "0.1.0";
              src = ./.;
              vendorHash = "sha256-iPragWnlZkyVc6UesGn18nsdt3+HXGEfT+mJtoevzMU=";
              subPackages = [ "cmd/mayuri" ];
              env.CGO_ENABLED = 0;
              ldflags = [
                "-s"
                "-w"
              ];
              nativeCheckInputs = [
                magick
                ffmpeg
              ];
              # subPackages would limit tests to cmd/mayuri; run the whole suite.
              checkPhase = ''
                runHook preCheck
                go test ./...
                runHook postCheck
              '';
              meta.mainProgram = "mayuri";
            };

            # `nix flake check` builds (and tests) the package, runs the linters,
            # validates the specs and tests the NixOS module in a VM.
            checks =
              lib.optionalAttrs (system == "x86_64-linux") {
                # services.mayuri2000 end to end in a VM (needs KVM).
                nixos-module = import ./nix/test.nix {
                  inherit pkgs;
                  quadletModule = inputs.quadlet-nix.nixosModules.quadlet;
                  mayuriModule = inputs.self.nixosModules.default;
                };
              }
              // {
                nixos-module-rejects-env-secrets = import ./nix/assertion-check.nix {
                  inherit pkgs lib system;
                  inherit (inputs) nixpkgs;
                  quadletModule = inputs.quadlet-nix.nixosModules.quadlet;
                  mayuriModule = inputs.self.nixosModules.default;
                };
                package = self'.packages.default;
                lint = self'.packages.default.overrideAttrs (old: {
                  pname = "mayuri2000-lint";
                  nativeBuildInputs = old.nativeBuildInputs ++ [ pkgs.golangci-lint ];
                  buildPhase = ''
                    runHook preBuild
                    export HOME=$TMPDIR GOLANGCI_LINT_CACHE=$TMPDIR/golangci-lint
                    golangci-lint run ./...
                    runHook postBuild
                  '';
                  doCheck = false;
                  installPhase = "touch $out";
                });
                # Behaviour specs (openspec/specs) must stay well-formed.
                specs = pkgs.runCommand "mayuri2000-specs" { nativeBuildInputs = [ pkgs.openspec ]; } ''
                  export HOME=$TMPDIR OPENSPEC_TELEMETRY=0
                  cd ${./.}
                  openspec validate --specs --strict --no-interactive
                  touch $out
                '';
              };

            packages.image = pkgs.dockerTools.buildLayeredImage {
              name = "mayuri2000";
              tag = "latest";
              contents = [
                pkgs.cacert
                pkgs.tzdata
              ];
              # Run as an unprivileged user (see the deployment spec). The passwd
              # entries keep getpwuid-style lookups working; /data is owned by the
              # user so new named volumes inherit that ownership.
              extraCommands = ''
                mkdir -p data tmp etc
                chmod 1777 tmp
                echo 'root:x:0:0::/root:/sbin/nologin' > etc/passwd
                echo 'mayuri:x:10001:10001::/tmp:/sbin/nologin' >> etc/passwd
                echo 'root:x:0:' > etc/group
                echo 'mayuri:x:10001:' >> etc/group
              '';
              fakeRootCommands = "chown 10001:10001 data";
              config = {
                User = "10001:10001";
                Entrypoint = [ (pkgs.lib.getExe self'.packages.default) ];
                Env = [
                  "HOME=/tmp"
                  "MAGICK_PATH=${pkgs.lib.getExe' magick "magick"}"
                  "FFMPEG_PATH=${pkgs.lib.getExe' ffmpeg "ffmpeg"}"
                  "SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
                  "DATABASE_PATH=/data/mayuri.db"
                  "TMPDIR=/tmp"
                ];
                Volumes."/data" = { };
              };
            };

            devShells.default = pkgs.mkShell {
              packages = [
                pkgs.go
                pkgs.gopls
                pkgs.golangci-lint
                pkgs.sqlite
                ffmpeg
                pkgs.openspec
                magick
              ];
            };
          };
      }
    );
}
