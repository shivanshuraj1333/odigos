//go:build integration

// Runs the real effective-config watcher against a live kind cluster and asserts
// that writing profiling.ui into the effective-config ConfigMap drives
// ProfileStore.Reconfigure live — the settings-page → live-tuning k8s path.
// Run with: go test -tags integration -run TestProfilingConfigWatcher_Kind ./frontend/kube/watchers/
package watchers

import (
	"context"
	"testing"
	"time"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/common/profilecache"
	"github.com/odigos-io/odigos/frontend/services/profiles"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func boolPtr(b bool) *bool { return &b }

func TestProfilingConfigWatcher_Kind(t *testing.T) {
	const ns = "odigos-system"

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: "kind-odigos-live"},
	).ClientConfig()
	if err != nil {
		t.Fatalf("kubeconfig: %v", err)
	}

	sch := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	cl, err := client.New(cfg, client.Options{Scheme: sch})
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = cl.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
	t.Cleanup(func() {
		_ = cl.Delete(context.Background(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: consts.OdigosEffectiveConfigName},
		})
	})

	c, err := cache.New(cfg, cache.Options{Scheme: sch, DefaultNamespaces: map[string]cache.Config{ns: {}}})
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	go func() { _ = c.Start(ctx) }()
	if !c.WaitForCacheSync(ctx) {
		t.Fatal("cache sync failed")
	}

	store := profilecache.NewStore(24, 120, 8<<20, time.Minute)
	gate := profiles.NewProfilesIngestGate(false)
	if err := StartProfilingConfigWatcher(ctx, c, ns, gate, store); err != nil {
		t.Fatalf("watcher: %v", err)
	}

	writeUI := func(maxSlots, slotMaxBytes, ttl int) {
		oc := &common.OdigosConfiguration{
			Profiling: &common.ProfilingConfiguration{
				Enabled: boolPtr(true),
				Ui:      &common.ProfilingUiConfiguration{MaxSlots: maxSlots, SlotMaxBytes: slotMaxBytes, SlotTTLSeconds: ttl},
			},
		}
		raw, err := yaml.Marshal(oc)
		if err != nil {
			t.Fatal(err)
		}
		cm := &corev1.ConfigMap{}
		key := client.ObjectKey{Namespace: ns, Name: consts.OdigosEffectiveConfigName}
		if err := cl.Get(ctx, key, cm); apierrors.IsNotFound(err) {
			if err := cl.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: consts.OdigosEffectiveConfigName},
				Data:       map[string]string{consts.OdigosConfigurationFileName: string(raw)},
			}); err != nil {
				t.Fatalf("create cm: %v", err)
			}
			return
		} else if err != nil {
			t.Fatalf("get cm: %v", err)
		}
		cm.Data = map[string]string{consts.OdigosConfigurationFileName: string(raw)}
		if err := cl.Update(ctx, cm); err != nil {
			t.Fatalf("update cm: %v", err)
		}
	}

	waitFor := func(field func() int, want int) {
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if field() == want {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("timed out: got %d want %d", field(), want)
	}

	// First write → watcher Reconfigures the store to the profiling.ui limits.
	writeUI(30, 4<<20, 90)
	waitFor(func() int { return store.MemoryStats().MaxSlots }, 30)
	if ms := store.MemoryStats(); ms.SlotMaxBytes != 4<<20 || ms.SlotTTLSeconds != 90 {
		t.Fatalf("first apply wrong: %+v", ms)
	}
	t.Logf("first apply: maxSlots=%d slotMaxBytes=%d ttl=%d", store.MemoryStats().MaxSlots, store.MemoryStats().SlotMaxBytes, store.MemoryStats().SlotTTLSeconds)

	// Update → watcher re-applies live (no restart).
	writeUI(60, 2<<20, 45)
	waitFor(func() int { return store.MemoryStats().MaxSlots }, 60)
	ms := store.MemoryStats()
	if ms.SlotMaxBytes != 2<<20 || ms.SlotTTLSeconds != 45 {
		t.Fatalf("live update wrong: %+v", ms)
	}
	t.Logf("live update: maxSlots=%d slotMaxBytes=%d ttl=%d (reconfigured live, no restart)", ms.MaxSlots, ms.SlotMaxBytes, ms.SlotTTLSeconds)
}
