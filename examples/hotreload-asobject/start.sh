#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

kubectl apply -f "$script_dir/configmap1.yaml"

# Match the name and namespace in the example manifests.
export NAMESPACE=default
export CONFIGMAP_NAME=app-config

cd "$script_dir"
echo "Watching initial values (interval: 10, retries: 3). Run reload.sh in another terminal."
exec go run .
