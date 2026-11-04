#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

kubectl apply -f "$script_dir/configmap2.yaml"
echo "Applied updated items: item1 (disabled: false, cpu: 100m, memory: 128Mi), item2 (disabled: true, cpu: 150m, memory: 512Mi). Check the watcher terminal for the unmarshalled items."
