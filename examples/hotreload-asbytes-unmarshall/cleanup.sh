#!/usr/bin/env bash
set -euo pipefail

kubectl delete configmap app-config --namespace default --ignore-not-found
