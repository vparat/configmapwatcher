package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/vparat/configmapwatcher"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// main logs updates to a ConfigMap Data key until interrupted.
// It takes no arguments and returns no values; settings come from the environment.
func main() {
	namespace := env("NAMESPACE", "default")
	name := env("CONFIGMAP_NAME", "app-config")
	key := env("CONFIGMAP_KEY", "config.yaml")

	// The default loader reads KUBECONFIG, falling back to ~/.kube/config
	// and in-cluster credentials when available.
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	).ClientConfig()

	if err != nil {
		log.Fatalf("load Kubernetes config: %v", err)
	}

	client, err := kubernetes.NewForConfig(config)

	if err != nil {
		log.Fatalf("create Kubernetes client: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	updates, shutdown, err := configmapwatcher.WatchAsBytes(ctx, client, namespace, name, key)

	if err != nil {
		log.Fatalf("watch ConfigMap: %v", err)
	}

	defer shutdown()
	log.Printf("watching %s/%s Data[%q]", namespace, name, key)

	// Cancellation closes updates. shutdown waits for the watcher to stop.
	for data := range updates {
		// Replace this with parsing and applying your application's configuration.
		// A missing key yields empty bytes; deletion yields the last known value.
		log.Printf("configuration update: %s", data)
	}

}

// env reads the environment variable named by key.
// It returns its nonempty value, or fallback when the variable is unset or empty.
func env(key, fallback string) string {

	if value := os.Getenv(key); value != "" {

		return value
	}

	return fallback
}
