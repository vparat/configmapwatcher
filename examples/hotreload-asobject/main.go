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

// main logs ConfigMap object updates until interrupted.
// It takes no arguments and returns no values; settings come from the environment.
func main() {
	namespace := env("NAMESPACE", "default")
	name := env("CONFIGMAP_NAME", "app-config")

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

	updates, shutdown, err := configmapwatcher.Watch(ctx, client, namespace, name)

	if err != nil {
		log.Fatalf("watch ConfigMap: %v", err)
	}

	defer shutdown()
	log.Printf("watching %s/%s", namespace, name)

	// Watch leaves updates open on cancellation, so listen to ctx.Done().
	for {
		select {
		case <-ctx.Done():
			return
		case cm := <-updates:
			// Each notification is a copy of the ConfigMap object.
			// Deletion carries the last known object.
			log.Printf("ConfigMap update: %s/%s resourceVersion=%s Data=%v",
				cm.Namespace, cm.Name, cm.ResourceVersion, cm.Data)
		}
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
