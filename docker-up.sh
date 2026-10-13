#!/usr/bin/env bash
# Start the Docker Compose example, building and loading the Nix-built image
# first if it isn't present locally. Extra arguments go to `docker compose up`.
#   ./docker-up.sh            # docker compose up -d
#   ./docker-up.sh --force-recreate
set -euo pipefail
cd "$(dirname "$0")"

image=mayuri2000:latest

if ! docker image inspect "$image" >/dev/null 2>&1; then
	echo "$image not found locally; building it with Nix..." >&2
	docker load <"$(nix build .#image --no-link --print-out-paths)"
fi

if [[ ! -f .env ]]; then
	echo "No .env file: copy env.example to .env and set DISCORD_TOKEN first." >&2
	exit 1
fi

docker compose up -d "$@"
