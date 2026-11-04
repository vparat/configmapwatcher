#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

kubectl apply -f "$script_dir/configmap1.yaml"

# Match the name, namespace, and key in the example manifests.
export NAMESPACE=default
export CONFIGMAP_NAME=app-config
export CONFIGMAP_KEY=config.yaml

cd "$script_dir"
echo "Watching unmarshalled items: item1 (disabled: false, cpu: 10m, memory: 64Mi), item2 (disabled: true, cpu: 100m, memory: 128Mi). Run reload.sh in another terminal."
exec go run .
