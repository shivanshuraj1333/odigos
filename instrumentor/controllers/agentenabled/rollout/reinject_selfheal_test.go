package rollout

import (
	"testing"
	"time"

	odigosv1alpha1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Tests for the self-healing re-roll guards used in Do()'s "rollout finished"
// branch (see the hardening comment there): a rollout that reports done but left
// pods the webhook never mutated is re-issued, bounded by these two predicates.

func TestWorkloadHasUninjectedPods(t *testing.T) {
	cases := []struct {
		name string
		ic   *odigosv1alpha1.InstrumentationConfig
		want bool
	}{
		{
			name: "nil status -> false (no info yet, do not re-roll)",
			ic:   &odigosv1alpha1.InstrumentationConfig{},
			want: false,
		},
		{
			name: "all pods injected -> false",
			ic: &odigosv1alpha1.InstrumentationConfig{
				Status: odigosv1alpha1.InstrumentationConfigStatus{
					PodsManifestInjectionStatus: &odigosv1alpha1.PodsManifestInjectionStatus{
						HasInjectedUpToDatePods: true,
					},
				},
			},
			want: false,
		},
		{
			name: "an uninjected pod present -> true (the webhook-race signature)",
			ic: &odigosv1alpha1.InstrumentationConfig{
				Status: odigosv1alpha1.InstrumentationConfigStatus{
					PodsManifestInjectionStatus: &odigosv1alpha1.PodsManifestInjectionStatus{
						HasInjectedUpToDatePods: true,
						HasUninjectedPods:       true,
					},
				},
			},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := workloadHasUninjectedPods(tc.ic); got != tc.want {
				t.Fatalf("workloadHasUninjectedPods = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReinjectCooldownElapsed(t *testing.T) {
	// never rolled -> elapsed (allow the first re-roll immediately)
	if !reinjectCooldownElapsed(&odigosv1alpha1.InstrumentationConfig{}) {
		t.Fatal("nil InstrumentationTime should be treated as cooldown elapsed")
	}
	// just rolled -> NOT elapsed (rate-limit prevents tight churn)
	now := metav1.NewTime(time.Now())
	icNow := &odigosv1alpha1.InstrumentationConfig{
		Status: odigosv1alpha1.InstrumentationConfigStatus{InstrumentationTime: &now},
	}
	if reinjectCooldownElapsed(icNow) {
		t.Fatal("a just-recorded rollout should NOT have its cooldown elapsed")
	}
	// rolled long ago -> elapsed
	old := metav1.NewTime(time.Now().Add(-2 * reinjectRetryCooldown))
	icOld := &odigosv1alpha1.InstrumentationConfig{
		Status: odigosv1alpha1.InstrumentationConfigStatus{InstrumentationTime: &old},
	}
	if !reinjectCooldownElapsed(icOld) {
		t.Fatal("a rollout older than the cooldown should be elapsed")
	}
}
