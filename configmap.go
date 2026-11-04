// Package configmapwatcher watches a selected Kubernetes ConfigMap.
package configmapwatcher

import (
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// objectWatch starts a ConfigMap watch whose shutdown function waits for cleanup.
// Inputs are the parent context, Kubernetes client, namespace, and ConfigMap name.
// Outputs are an object channel, a shutdown callback with no inputs or outputs,
// and a registration error. The channel remains open when the watch stops.
type objectWatch func(context.Context, kubernetes.Interface, string, string) (<-chan *corev1.ConfigMap, func(), error)

// Watch emits copies of the named ConfigMap on addition, Data changes, and
// deletion, including recreation. Deletions carry the last known object.
// The informer reconnects and relists when its API watch expires.
//
// Canceling ctx stops the watch but leaves the channel open. The returned
// shutdown function cancels the watch and waits for the informer to stop.
// Registration failures return nil outputs and an error.
// API list and watch failures occur asynchronously through the informer.
//
// Parameters:
//   - ctx: controls the lifetime of the watch.
//   - client: Kubernetes client used to list and watch ConfigMaps.
//   - namespace: namespace containing the ConfigMap.
//   - name: name of the ConfigMap to watch.
//
// Returns, in order:
//   - updates: receive-only channel of ConfigMap copies; left open on shutdown.
//   - shutdown: cancels the watch and waits for the informer to stop; takes no
//     arguments and returns no values.
//   - err: handler registration error, or nil on successful registration.
func Watch(
	ctx context.Context,
	client kubernetes.Interface,
	namespace string,
	name string,
) (<-chan *corev1.ConfigMap, func(), error) {
	factory := informers.NewSharedInformerFactoryWithOptions(
		client,
		0,
		informers.WithNamespace(namespace),
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.FieldSelector = fields.OneTermEqualSelector("metadata.name", name).String()
		}),
	)

	return watch(
		ctx,
		factory,
		factory.Core().V1().ConfigMaps().Informer(),
		namespace,
		name,
	)
}

// WatchAsBytes forwards Watch notifications as bytes from the selected Data key.
// A missing key produces empty bytes; deletion carries the last known value.
// Any Data change triggers forwarding, even if key's value is unchanged.
//
// The output channel closes when forwarding stops, retaining buffered values.
// The returned shutdown function cancels forwarding and waits for both it and
// the informer to stop. Registration failures return nil outputs and an error.
//
// Parameters:
//   - ctx: controls the lifetime of the watch and byte forwarding.
//   - client: Kubernetes client used to list and watch ConfigMaps.
//   - namespace: namespace containing the ConfigMap.
//   - name: name of the ConfigMap to watch.
//   - key: entry in ConfigMap.Data to convert to bytes.
//
// Returns, in order:
//   - updates: receive-only byte channel; closes when forwarding stops.
//   - shutdown: cancels forwarding and waits for it and the informer to stop;
//     takes no arguments and returns no values.
//   - err: watch startup error, or nil on successful registration.
func WatchAsBytes(
	ctx context.Context,
	client kubernetes.Interface,
	namespace string,
	name string,
	key string,
) (<-chan []byte, func(), error) {

	return watchAsBytes(
		ctx,
		client,
		namespace,
		name,
		key,
		Watch,
	)
}

// watchAsBytes converts notifications from start into values for key.
// Its shutdown function stops the object watch and waits for byte forwarding.
//
// Parameters:
//   - ctx: parent context for the watch and byte forwarding.
//   - client: Kubernetes client passed to start.
//   - namespace: ConfigMap namespace passed to start.
//   - name: ConfigMap name passed to start.
//   - key: entry in ConfigMap.Data to convert to bytes.
//   - start: starts the object watch; its channel must remain open even on shutdown.
//
// Returns, in order:
//   - updates: receive-only byte channel that closes when forwarding stops.
//   - shutdown: cancels and waits for forwarding and the object watch; takes no
//     arguments and returns no values.
//   - err: wrapped startup error with nil updates and shutdown, or nil on success.
func watchAsBytes(
	ctx context.Context,
	client kubernetes.Interface,
	namespace string,
	name string,
	key string,
	start objectWatch,
) (<-chan []byte, func(), error) {
	watchCtx, cancel := context.WithCancel(ctx)
	objects, shutdown, err := start(
		watchCtx,
		client,
		namespace,
		name,
	)

	if err != nil {
		cancel()

		return nil, nil, fmt.Errorf("start configmap watch %s/%s: %w", namespace, name, err)
	}

	updates := make(chan []byte, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(updates)

		for {
			select {
			case <-watchCtx.Done():
				return
			case cm := <-objects:
				select {
				case updates <- []byte(cm.Data[key]):
				case <-watchCtx.Done():
					return
				}
			}
		}

	}()

	return updates, func() {
		cancel()
		shutdown()
		<-done
	}, nil
}

// watch registers handlers on informer and starts factory.
// Notifications are copied so callers cannot modify the informer cache.
//
// Parameters:
//   - ctx: parent context controlling the informer lifetime.
//   - factory: factory to start after registering handlers and stop on shutdown.
//   - informer: ConfigMap informer belonging to factory that receives handlers.
//   - namespace: namespace used to filter notifications.
//   - name: ConfigMap name used to filter notifications.
//
// Returns, in order:
//   - updates: receive-only channel of ConfigMap copies; left open on shutdown.
//   - shutdown: cancels the watch and waits for factory shutdown; takes no
//     arguments and returns no values.
//   - err: wrapped registration error with nil updates and shutdown, or nil on success.
func watch(
	ctx context.Context,
	factory informers.SharedInformerFactory,
	informer cache.SharedIndexInformer,
	namespace string,
	name string,
) (<-chan *corev1.ConfigMap, func(), error) {
	updates := make(chan *corev1.ConfigMap, 1)
	watchCtx, cancel := context.WithCancel(ctx)

	enqueue := func(obj any) {

		if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
			obj = tombstone.Obj
		}

		cm, ok := obj.(*corev1.ConfigMap)
		matches := ok && cm.Name == name && cm.Namespace == namespace

		if !matches {
			return
		}

		select {
		case updates <- cm.DeepCopy():
		case <-watchCtx.Done():
		}
	}

	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    enqueue,
		DeleteFunc: enqueue,
		UpdateFunc: func(old, next any) {

			if !maps.Equal(old.(*corev1.ConfigMap).Data, next.(*corev1.ConfigMap).Data) {
				enqueue(next)
			}

		},
	}); err != nil {
		cancel()

		return nil, nil, fmt.Errorf("register configmap handler %s/%s: %w", namespace, name, err)
	}

	factory.Start(watchCtx.Done())

	return updates, func() {
		cancel()
		factory.Shutdown()
	}, nil
}
