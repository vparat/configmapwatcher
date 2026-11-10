# ConfigMap hot reload example

This example uses `configmapwatcher.Watch` to receive copies of the
`default/app-config` ConfigMap object on addition, Data changes, and deletion.
It logs the object's namespace, name, resource version, and Data without
restarting the program. The example does not parse or apply the integer settings.
Deletion carries the last known object. Cancellation leaves the updates channel
open, so the program listens for context cancellation and calls shutdown on exit.

## Prerequisites

- Go 1.27.4 or later, Bash, and `kubectl`.
- Access to a Kubernetes cluster through your current kubeconfig context.
- Permission to create, patch, get, list, and watch ConfigMaps in `default`,
  plus delete permission for cleanup.

All scripts use your current Kubernetes context. Set `KUBECONFIG` in both
terminals if you need a specific kubeconfig file.

## Run the demo

Run these commands from this directory (`examples/hotreload-asobject`).

1. In the first terminal, start the example:

   ```bash
   ./start.sh
   ```

   This applies `configmap1.yaml` and starts the watcher. Wait for the initial
   configuration to appear within the logged `Data["config.yaml"]` value
   (the actual log uses Go map formatting, `Data=map[config.yaml:...]`):

   ```yaml
   interval: 10
   retries: 3
   ```

2. In a second terminal, open this same directory and apply the update:

   ```bash
   ./reload.sh
   ```

   This applies `configmap2.yaml` to the same ConfigMap. The running watcher
   logs the updated configuration:

   ```yaml
   interval: 20
   retries: 5
   ```

3. Press Ctrl+C in the first terminal to stop the watcher.

Run `./start.sh` again to reset the values and repeat the demo.

## Run manually

The program reads these environment variables; unset or empty values use the
defaults below:

| Variable | Default | Purpose |
| --- | --- | --- |
| `NAMESPACE` | `default` | ConfigMap namespace |
| `CONFIGMAP_NAME` | `app-config` | ConfigMap name |

The Kubernetes client uses its standard configuration loader, including
`KUBECONFIG`, `~/.kube/config`, and in-cluster credentials when available.

From this directory:

```bash
kubectl apply -f configmap1.yaml
NAMESPACE=default CONFIGMAP_NAME=app-config \
  go run .
```

`start.sh` explicitly sets the two watcher variables to match the supplied
manifests.

## Test against the local package

This example has its own `go.mod` with a local replacement:

```go
replace github.com/vparat/configmapwatcher => ../..
```

Running the example uses the package source at the repository root, including
your local changes. From this directory, run `go test ./...` to compile the
example. Run `go test ./...` separately at the repository root to test the
library; root module tests do not include this nested example module.

## Cleanup

After stopping the watcher, delete the example ConfigMap from this directory:

```bash
./cleanup.sh
```

The script deletes `app-config` in the `default` namespace and succeeds if it
is already absent. Both manifests refer to this same resource.
