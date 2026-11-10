# ConfigMap hot reload with YAML unmarshalling

This example uses `configmapwatcher.WatchAsBytes` to watch the `config.yaml`
Data key in the `default/app-config` ConfigMap. It logs the initial configuration
and subsequent changes without restarting the program. Each update is unmarshalled
with `go.yaml.in/yaml/v3` into a struct containing `Items []Item`, then the slice
is printed with `%+v` to include field names and nested `ItemsIn` values.

The YAML has a top-level `items` list. Each item contains `name`, `disabled`, and
an `items-in` object with `name`, `namespace`, `cpu`, and `memory`. YAML struct
tags map these keys to Go fields; CPU and memory quantities remain strings.

```go
var config struct {
    Items []Item `yaml:"items"`
}

if err := yaml.Unmarshal(data, &config); err != nil {
    log.Printf("unmarshal configuration: %v", err)
    continue
}

log.Printf("configuration items: %+v", config.Items)
```

This snippet runs inside the updates loop in `main.go`. Invalid YAML is logged
and skipped, and the watcher continues receiving updates. A missing Data key
produces empty bytes and prints an empty slice. Deletion repeats the last known
value. The example prints the parsed configuration without applying resource
settings to the cluster.

## Prerequisites

- Go 1.27.4 or later, Bash, and `kubectl`.
- Access to a Kubernetes cluster through your current kubeconfig context.
- Permission to create, patch, get, list, and watch ConfigMaps in `default`,
  plus delete permission for cleanup.

All scripts use your current Kubernetes context. Set `KUBECONFIG` in both
terminals if you need a specific kubeconfig file.

## Run the demo

Run these commands from this directory (`examples/hotreload-asbytes-unmarshall`).

1. In the first terminal, start the example:

   ```bash
   ./start.sh
   ```

   This applies `configmap1.yaml` and starts the watcher. Wait for the initial
   configuration to appear in the logs:

   The log line below omits the timestamp:

   ```text
   configuration items: [{Name:item1 Disabled:false ItemsIn:{Name:items1-in Namespace:default CPU:10m Memory:64Mi}} {Name:item2 Disabled:true ItemsIn:{Name:items2-in Namespace:default CPU:100m Memory:128Mi}}]
   ```

2. In a second terminal, open this same directory and apply the update:

   ```bash
   ./reload.sh
   ```

   This applies `configmap2.yaml` to the same ConfigMap. The running watcher
   logs the updated configuration:

   ```text
   configuration items: [{Name:item1 Disabled:false ItemsIn:{Name:items1-in Namespace:default CPU:100m Memory:128Mi}} {Name:item2 Disabled:true ItemsIn:{Name:items2-in Namespace:default CPU:150m Memory:512Mi}}]
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
| `CONFIGMAP_KEY` | `config.yaml` | Data key to watch |

The Kubernetes client uses its standard configuration loader, including
`KUBECONFIG`, `~/.kube/config`, and in-cluster credentials when available.

From this directory:

```bash
kubectl apply -f configmap1.yaml
NAMESPACE=default CONFIGMAP_NAME=app-config CONFIGMAP_KEY=config.yaml \
  go run .
```

`start.sh` explicitly sets the three watcher variables to match the supplied
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
