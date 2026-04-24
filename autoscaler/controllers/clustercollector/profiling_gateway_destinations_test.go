package clustercollector

import (
	"testing"

	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
	"github.com/odigos-io/odigos/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestProfileOtlpExporterNames(t *testing.T) {
	list := &odigosv1.DestinationList{Items: []odigosv1.Destination{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pyroscope"},
			Spec: odigosv1.DestinationSpec{
				Type:    common.GenericOTLPDestinationType,
				Signals: []common.ObservabilitySignal{common.ProfilesObservabilitySignal},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec: odigosv1.DestinationSpec{
				Type:    common.GenericOTLPDestinationType,
				Signals: []common.ObservabilitySignal{common.TracesObservabilitySignal},
			},
		},
	}}
	got := profileOtlpExporterNames(list)
	if len(got) != 1 || got[0] != "otlp/generic-pyroscope" {
		t.Fatalf("got %#v", got)
	}
}

func TestProfileOtlpExporterNames_SkipsDisabled(t *testing.T) {
	disabled := true
	list := &odigosv1.DestinationList{Items: []odigosv1.Destination{{
		ObjectMeta: metav1.ObjectMeta{Name: "x"},
		Spec: odigosv1.DestinationSpec{
			Type:     common.GenericOTLPDestinationType,
			Signals:  []common.ObservabilitySignal{common.ProfilesObservabilitySignal},
			Disabled: &disabled,
		},
	}}}
	if n := profileOtlpExporterNames(list); len(n) != 0 {
		t.Fatalf("expected empty, got %#v", n)
	}
}
