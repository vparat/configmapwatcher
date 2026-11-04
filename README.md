# configmapwatcher

A Go library for watching a single Kubernetes ConfigMap by namespace and name.
Receive copies of the ConfigMap object or bytes from a selected `Data` key to
reload application configuration when it changes.

## Installation

The current `go.mod` declares Go 1.27.3 as the minimum version.  While this code is 
compatible with lower versions of Go, the latest is recommended due to the high velocity 
of CVEs behing exposed by AI.

```bash
go get github.com/vparat/configmapwatcher
```

Supply a `kubernetes.Interface` client with credentials that can list and watch
ConfigMaps in the target namespace. The runnable examples show how to load
kubeconfig or in-cluster credentials.

## Usage

Import the package:

```go
import "github.com/vparat/configmapwatcher"
```

### Watch a Data key as bytes

Given a context `ctx` and Kubernetes client `client`:

```go
updates, shutdown, err := configmapwatcher.WatchAsBytes(
    ctx, client, "default", "app-config", "config.yaml",
)
if err != nil {
    return err
}
defer shutdown()

for data := range updates {
    log.Printf("configuration update: %s", data)
}

return nil
```

The selected value is converted to `[]byte`. A missing key produces empty bytes.
Canceling the context stops forwarding and closes the updates channel; buffered
values can still be read. Call `shutdown` to wait for cleanup.

### Watch ConfigMap objects

```go
updates, shutdown, err := configmapwatcher.Watch(
    ctx, client, "default", "app-config",
)
if err != nil {
    return err
}
defer shutdown()

for {
    select {
    case <-ctx.Done():
        return nil
    case cm := <-updates:
        log.Printf("ConfigMap %s/%s: %v", cm.Namespace, cm.Name, cm.Data)
    }
}
```

Each notification contains a copy of the ConfigMap, so consumers can modify it
without changing the informer cache. `Watch` leaves its channel open when the
context is canceled; use `ctx.Done()` to exit the receive loop. The returned
`shutdown` function stops the watch and waits for the informer to stop.

Both snippets belong inside a function returning `error`; they use the standard
library `log` package.

## Notification behavior

- Notifications are sent for the initial object if it exists, additions,
  changes to `Data`, deletion, and recreation.
- Metadata-only and `BinaryData`-only updates do not trigger notifications.
- Deletion sends the last known object or selected value. Notifications do not
  include an event type to distinguish deletion from other events.
- `WatchAsBytes` forwards all `Data` change notifications, including changes to
  keys other than the selected key.
- The informer reconnects and relists when the API watch expires.

Starting a watcher does not wait for its initial API list to succeed. Returned
errors cover handler registration; API connection and permission failures occur
asynchronously through the informer.

## Hot reload demos

The bytes and object demos apply a ConfigMap with `interval: 10` and `retries: 3`, then update it
to `interval: 20` and `retries: 5` while the watcher keeps running. They log
configuration updates without parsing or applying the integer settings.

The YAML unmarshalling demo decodes a top-level `items` list into `[]Item` and
prints the slice, including nested `items-in` fields. Its reload changes CPU and
memory values; invalid YAML is logged and skipped while the watcher continues.

| Example | API | Instructions |
| --- | --- | --- |
| Bytes | `WatchAsBytes` | [hotreload-asbytes](examples/hotreload-asbytes/README.md) |
| Bytes with YAML unmarshalling | `WatchAsBytes` | [hotreload-asbytes-unmarshall](examples/hotreload-asbytes-unmarshall/README.md) |
| Objects | `Watch` | [hotreload-asobject](examples/hotreload-asobject/README.md) |

With Bash, Go, `kubectl`, and a configured Kubernetes context, run from the
repository root in the first terminal:

```bash
./examples/hotreload-asbytes/start.sh
```

Wait for the initial configuration, then run in a second terminal:

```bash
./examples/hotreload-asbytes/reload.sh
```

Use `hotreload-asobject` in both commands for the object demo, or
`hotreload-asbytes-unmarshall` for the YAML unmarshalling demo. All demos use
`default/app-config`, so run them separately. Press Ctrl+C to stop the watcher.
To delete the example resource afterward:

```bash
./examples/hotreload-asbytes/cleanup.sh
```

Each example directory includes a `cleanup.sh` that deletes `default/app-config`
and succeeds if the ConfigMap is already absent.

## Local development

Each example is a separate Go module with a local replacement:

```go
replace github.com/vparat/configmapwatcher => ../..
```

This makes the examples use your local library changes. Run checks separately
for the library and each nested example module:

```bash
go test ./...
(cd examples/hotreload-asbytes && go test ./...)
(cd examples/hotreload-asbytes-unmarshall && go test ./...)
(cd examples/hotreload-asobject && go test ./...)
```

Run fuzzing for event filtering, object copy isolation, byte forwarding, and
shutdown behavior without a Kubernetes cluster:

```bash
make test-fuzz
# Override the default 30 seconds per target and two workers:
make test-fuzz FUZZTIME=1m FUZZ_PARALLEL=4
```

The seed cases also run during ordinary `go test ./...` and `make race` runs.
