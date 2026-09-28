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
        in
        {
          packages.default = pkgs.buildGoModule {
            pname = "mayuri2000";
            version = "0.1.0";
            src = ./.;
            vendorHash = "sha256-lB9jOFZbEdgR8zs+rbX3dnRBQMn9l0ASWZ++Lq7zJ/k=";
            subPackages = [ "cmd/mayuri" ];
            env.CGO_ENABLED = 0;
            ldflags = [
              "-s"
              "-w"
            ];
            nativeCheckInputs = [ magick ];
            # subPackages would limit tests to cmd/mayuri; run the whole suite.
            checkPhase = ''
              runHook preCheck
              go test ./...
              runHook postCheck
            '';
            meta.mainProgram = "mayuri";
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
              magick
            ];
          };
        };
    };
}
