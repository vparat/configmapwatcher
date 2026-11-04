package configmapwatcher

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8swatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func TestWatchDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "config", Namespace: "default"},
		Data:       map[string]string{"config.yaml": "events: []"},
	}
	client := fake.NewClientset(cm)
	events := k8swatch.NewRaceFreeFake()
	watching := make(chan struct{})
	client.PrependWatchReactor("configmaps", func(clienttesting.Action) (bool, k8swatch.Interface, error) {
		close(watching)

		return true, events, nil
	})
	updates, shutdown, err := Watch(
		ctx,
		client,
		cm.Namespace,
		cm.Name,
	)

	if err != nil {
		t.Fatal(err)
	}

	defer shutdown()
	receive := func() *corev1.ConfigMap {
		t.Helper()

		select {
		case got := <-updates:
			return got
		case <-ctx.Done():
			t.Fatal("timed out waiting for ConfigMap notification")

			return nil
		}
	}
	receive()

	select {
	case <-watching:
	case <-ctx.Done():
		t.Fatal("watch did not start")
	}

	events.Delete(cm)
	deleted := receive()

	if deleted.Name != cm.Name || deleted.Data["config.yaml"] != cm.Data["config.yaml"] {
		t.Fatalf("unexpected deletion notification: %+v", deleted)
	}

	deleted.Data["config.yaml"] = "changed"

	if cm.Data["config.yaml"] != "events: []" {
		t.Fatal("deletion notification was not copied")
	}

}

// handlerInformer captures callbacks without starting an API watch.
type handlerInformer struct {
	cache.SharedIndexInformer
	handler cache.ResourceEventHandler
	err     error
}

func (i *handlerInformer) AddEventHandler(
	handler cache.ResourceEventHandler,
) (cache.ResourceEventHandlerRegistration, error) {
	i.handler = handler

	return nil, i.err
}

func TestWatchHandlers(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	factory := informers.NewSharedInformerFactory(fake.NewClientset(), 0)
	informer := &handlerInformer{}
	updates, shutdown, err := watch(
		ctx,
		factory,
		informer,
		"default",
		"config",
	)

	if err != nil {
		t.Fatal(err)
	}

	defer shutdown()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "config", Namespace: "default"},
		Data:       map[string]string{"key": "initial"},
	}
	assertUpdate := func(want string) {
		t.Helper()

		select {
		case got := <-updates:

			if got.Data["key"] != want {
				t.Fatalf("data = %v, want %q", got.Data, want)
			}

			got.Data["key"] = "mutated"
		default:
			t.Fatal("missing notification")
		}
	}
	assertEmpty := func() {
		t.Helper()

		select {
		case got := <-updates:
			t.Fatalf("unexpected notification: %+v", got)
		default:
		}
	}
	informer.handler.OnAdd(cm, false)
	assertUpdate("initial")
	informer.handler.OnUpdate(cm, cm.DeepCopy())
	assertEmpty()
	next := cm.DeepCopy()
	next.Data["key"] = "updated"
	informer.handler.OnUpdate(cm, next)
	assertUpdate("updated")
	informer.handler.OnDelete(cache.DeletedFinalStateUnknown{Obj: next})
	assertUpdate("updated")

	if next.Data["key"] != "updated" || cm.Data["key"] != "initial" {
		t.Fatal("notification modified original object")
	}

	informer.handler.OnDelete(cache.DeletedFinalStateUnknown{Obj: "invalid"})
	informer.handler.OnAdd(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "default"},
	}, false)
	informer.handler.OnAdd(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "config", Namespace: "other"},
	}, false)
	assertEmpty()

	// Fill the buffer so a canceled callback cannot send another notification.
	informer.handler.OnAdd(cm, false)
	cancel()
	done := make(chan struct{})
	go func() {
		informer.handler.OnDelete(cm)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled callback blocked on a full channel")
	}

	assertUpdate("initial")
	assertEmpty()
}

func TestWatchRegistrationError(t *testing.T) {
	want := errors.New("registration failed")
	factory := informers.NewSharedInformerFactory(fake.NewClientset(), 0)
	informer := &handlerInformer{err: want}
	updates, shutdown, err := watch(
		t.Context(),
		factory,
		informer,
		"default",
		"config",
	)

	unexpectedError := !errors.Is(err, want)
	unexpectedOutputs := updates != nil || shutdown != nil

	if unexpectedError || unexpectedOutputs {
		t.Fatalf(
			"watch returned (%v, %v, %v), want nil channels and registration error",
			updates,
			shutdown == nil,
			err,
		)
	}

}

func TestWatchAsBytes(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "config", Namespace: "default"},
		Data:       map[string]string{"config.yaml": "events: []", "other": "ignored"},
	}
	client := fake.NewClientset(cm)
	updates, shutdown, err := WatchAsBytes(
		ctx,
		client,
		cm.Namespace,
		cm.Name,
		"config.yaml",
	)

	if err != nil {
		t.Fatal(err)
	}

	defer shutdown()
	assertBytes := func(want string) {
		t.Helper()

		select {
		case got, ok := <-updates:

			if !ok || string(got) != want {
				t.Fatalf(
					"notification = %q, open = %v, want %q",
					got,
					ok,
					want,
				)
			}

		case <-ctx.Done():
			t.Fatal("timed out waiting for bytes")
		}
	}
	assertBytes("events: []")
	cm.Data["config.yaml"] = "events: [{name: updated}]"

	if _, err := client.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	assertBytes(cm.Data["config.yaml"])
	delete(cm.Data, "config.yaml")

	if _, err := client.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	assertBytes("")
	shutdown()

	if _, ok := <-updates; ok {
		t.Fatal("bytes channel remained open after shutdown")
	}

}

func TestWatchAsBytesRegistrationError(t *testing.T) {
	want := errors.New("registration failed")
	var watchCtx context.Context
	start := func(ctx context.Context, _ kubernetes.Interface, _, _ string) (<-chan *corev1.ConfigMap, func(), error) {
		watchCtx = ctx

		return nil, nil, want
	}
	updates, shutdown, err := watchAsBytes(
		t.Context(),
		fake.NewClientset(),
		"default",
		"config",
		"key",
		start,
	)

	unexpectedError := !errors.Is(err, want)
	unexpectedOutputs := updates != nil || shutdown != nil

	if unexpectedError || unexpectedOutputs {
		t.Fatalf(
			"got (%v, shutdown nil = %v, %v), want nil outputs and registration error",
			updates,
			shutdown == nil,
			err,
		)
	}

	if !errors.Is(watchCtx.Err(), context.Canceled) {
		t.Fatal("failed watch context was not canceled")
	}

}

func TestWatchAsBytesBlockedOutputCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	objects := make(chan *corev1.ConfigMap)
	var stopped bool
	start := func(context.Context, kubernetes.Interface, string, string) (<-chan *corev1.ConfigMap, func(), error) {
		return objects, func() { stopped = true }, nil
	}
	updates, shutdown, err := watchAsBytes(
		ctx,
		fake.NewClientset(),
		"default",
		"config",
		"key",
		start,
	)

	if err != nil {
		t.Fatal(err)
	}

	defer shutdown()
	cm := &corev1.ConfigMap{Data: map[string]string{"key": "value"}}

	// The second receive proves that the output buffer holds the first value.
	for range 2 {
		select {
		case objects <- cm:
		case <-ctx.Done():
			t.Fatal("byte forwarding did not receive object")
		}
	}

	cancel()
	shutdown()

	if !stopped {
		t.Fatal("underlying watch was not stopped")
	}

	got, ok := <-updates

	if !ok || string(got) != "value" {
		t.Fatalf("buffered value = %q, open = %v", got, ok)
	}

	if _, ok := <-updates; ok {
		t.Fatal("output remained open after cancellation")
	}

}

// FuzzWatchHandlers uses f to generate ConfigMap identities, Data, and event types.
// It checks filtering, change detection, and copy isolation; it returns no values.
func FuzzWatchHandlers(f *testing.F) {
	f.Add("default", "config", "key", "initial", "updated", uint8(0))
	f.Add("", "", "", "", "", uint8(1))
	f.Add("default", "config", "key", "value", "value", uint8(2))
	f.Add("default", "config", "key", "old", "new", uint8(3))
	f.Add("default", "config", "key", "old", "new", uint8(4))
	f.Add("default", "config", "key", "old", "new", uint8(5))
	f.Add("default", "config", "key", "old", "new", uint8(6))
	f.Add("default", "config", "key", "old", "new", uint8(7))
	f.Fuzz(func(t *testing.T, namespace, name, key, value, nextValue string, event uint8) {
		factory := informers.NewSharedInformerFactory(fake.NewClientset(), 0)
		informer := &handlerInformer{}
		updates, shutdown, err := watch(t.Context(), factory, informer, namespace, name)

		if err != nil {
			t.Fatal(err)
		}

		defer shutdown()
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Data:       map[string]string{key: value},
		}
		next := cm.DeepCopy()
		want := cm.DeepCopy()
		expectUpdate := true

		switch event % 8 {
		case 0:
			informer.handler.OnAdd(cm, false)
		case 1:
			next.Data[key] = nextValue
			want = next.DeepCopy()
			expectUpdate = value != nextValue
			informer.handler.OnUpdate(cm, next)
		case 2:
			informer.handler.OnDelete(cm)
		case 3:
			informer.handler.OnDelete(cache.DeletedFinalStateUnknown{Obj: cm})
		case 4:
			next.Name += "-other"
			expectUpdate = false
			informer.handler.OnAdd(next, false)
		case 5:
			next.Namespace += "-other"
			expectUpdate = false
			informer.handler.OnDelete(cache.DeletedFinalStateUnknown{Obj: next})
		case 6:
			expectUpdate = false
			informer.handler.OnDelete(cache.DeletedFinalStateUnknown{Obj: value})
		case 7:
			next.Labels = map[string]string{key: nextValue}
			expectUpdate = false
			informer.handler.OnUpdate(cm, next)
		}

		select {
		case got := <-updates:

			if !expectUpdate {
				t.Fatalf("unexpected notification for event %d: %+v", event%8, got)
			}

			if got.Namespace != namespace || got.Name != name || !maps.Equal(got.Data, want.Data) {
				t.Fatalf("notification = %+v, want %+v", got, want)
			}

			got.Data[key] += "-mutated"

			if cm.Data[key] != value || next.Data[key] != want.Data[key] {
				t.Fatal("notification mutation changed the source object")
			}

		default:

			if expectUpdate {
				t.Fatalf("missing notification for event %d", event%8)
			}

		}
	})
}

// FuzzWatchAsBytes uses f to generate keys, raw string values, and key presence.
// It checks byte preservation, missing keys, and shutdown; it returns no values.
func FuzzWatchAsBytes(f *testing.F) {
	f.Add("config.yaml", "interval: 10\nretries: 3\n", true)
	f.Add("", "", true)
	f.Add("missing", "ignored", false)
	f.Add("key", "\x00\xff\xfe", true)
	f.Fuzz(func(t *testing.T, key, value string, present bool) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		objects := make(chan *corev1.ConfigMap, 1)
		cm := &corev1.ConfigMap{}
		want := ""

		if present {
			cm.Data = map[string]string{key: value}
			want = value
		}

		objects <- cm
		stopped := false
		start := func(context.Context, kubernetes.Interface, string, string) (<-chan *corev1.ConfigMap, func(), error) {

			return objects, func() { stopped = true }, nil
		}
		updates, shutdown, err := watchAsBytes(ctx, nil, "default", "config", key, start)

		if err != nil {
			t.Fatal(err)
		}

		defer shutdown()

		select {
		case got, ok := <-updates:

			if !ok || string(got) != want {
				t.Fatalf("bytes = %q, open = %v, want %q", got, ok, want)
			}

			// The output is caller-owned and must not change the original Data.
			for i := range got {
				got[i] ^= 0xff
			}

			if present && cm.Data[key] != value {
				t.Fatal("byte mutation changed ConfigMap Data")
			}

		case <-ctx.Done():
			t.Fatal("timed out waiting for bytes")
		}

		shutdown()

		if !stopped {
			t.Fatal("shutdown did not stop the object watch")
		}

		if _, ok := <-updates; ok {
			t.Fatal("byte channel remained open after shutdown")
		}

	})
}
