package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/vparat/configmapwatcher"
	"go.yaml.in/yaml/v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Item struct {
	Name     string  `yaml:"name"`
	Disabled bool    `yaml:"disabled"`
	ItemsIn  ItemsIn `yaml:"items-in"`
}

type ItemsIn struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
	CPU       string `yaml:"cpu"`
	Memory    string `yaml:"memory"`
}

// main unmarshals updates to a ConfigMap Data key and prints the item slice until interrupted.
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
		// A missing key yields empty bytes; deletion yields the last known value.
		// The YAML document wraps the item slice in a top-level "items" field.
		var config struct {
			Items []Item `yaml:"items"`
		}

		if err := yaml.Unmarshal(data, &config); err != nil {
			log.Printf("unmarshal configuration: %v", err)
			continue
		}

		log.Printf("configuration items: %+v", config.Items)
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
