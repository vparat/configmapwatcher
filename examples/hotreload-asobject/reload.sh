#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

kubectl apply -f "$script_dir/configmap2.yaml"
echo "Applied updated values (interval: 20, retries: 5). Check the watcher terminal."
