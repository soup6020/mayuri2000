{
  description = "Mayuri2000 - a Discord bot built on disgo";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };

  outputs =
    inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];

      perSystem =
        { pkgs, self', ... }:
        let
          magick = pkgs.imagemagick.override { liblqr1Support = true; };
          ffmpeg = pkgs.ffmpeg-headless;
        in
        {
          packages.default = pkgs.buildGoModule {
            pname = "mayuri2000";
            version = "0.1.0";
            src = ./.;
            vendorHash = "sha256-frcEMdhcyKqDusy7EkA1p3ZjlQsOIdOMJn3BbF7UAvQ=";
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

          # `nix flake check` builds (and tests) the package and runs the linters.
          checks = {
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
            extraCommands = "mkdir -p data tmp && chmod 1777 tmp";
            config = {
              Entrypoint = [ (pkgs.lib.getExe self'.packages.default) ];
              Env = [
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
    };
}
