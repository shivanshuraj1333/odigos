package utils

import (
	"context"
	"testing"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestProfilingFromEffectiveOrHelm_UsesEffectiveWhenNonNil(t *testing.T) {
	t.Setenv(consts.CurrentNamespaceEnvVar, consts.DefaultOdigosNamespace)
	on := true
	effective := &common.ProfilingConfiguration{Enabled: &on}
	c := fake.NewClientBuilder().Build()
	got, err := ProfilingFromEffectiveOrHelm(context.Background(), c, effective)
	require.NoError(t, err)
	assert.Same(t, effective, got)
}

func TestProfilingFromEffectiveOrHelm_FallsBackToHelmWhenNil(t *testing.T) {
	t.Setenv(consts.CurrentNamespaceEnvVar, consts.DefaultOdigosNamespace)
	helmYAML := `configVersion: 1
telemetryEnabled: true
profiling:
  enabled: true
`
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consts.OdigosConfigurationName,
			Namespace: consts.DefaultOdigosNamespace,
		},
		Data: map[string]string{
			consts.OdigosConfigurationFileName: helmYAML,
		},
	}
	c := fake.NewClientBuilder().WithObjects(cm).Build()
	got, err := ProfilingFromEffectiveOrHelm(context.Background(), c, nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Enabled)
	assert.True(t, *got.Enabled)
}

func TestProfilingFromEffectiveOrHelm_ExplicitOffInEffectiveNotOverridden(t *testing.T) {
	t.Setenv(consts.CurrentNamespaceEnvVar, consts.DefaultOdigosNamespace)
	off := false
	effective := &common.ProfilingConfiguration{Enabled: &off}
	helmYAML := `configVersion: 1
telemetryEnabled: true
profiling:
  enabled: true
`
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consts.OdigosConfigurationName,
			Namespace: consts.DefaultOdigosNamespace,
		},
		Data: map[string]string{
			consts.OdigosConfigurationFileName: helmYAML,
		},
	}
	c := fake.NewClientBuilder().WithObjects(cm).Build()
	got, err := ProfilingFromEffectiveOrHelm(context.Background(), c, effective)
	require.NoError(t, err)
	require.NotNil(t, got.Enabled)
	assert.False(t, *got.Enabled)
}

func TestProfilingFromEffectiveOrHelm_MergesGatewayUiOtlpEndpointFromHelm(t *testing.T) {
	t.Setenv(consts.CurrentNamespaceEnvVar, consts.DefaultOdigosNamespace)
	on := true
	effective := &common.ProfilingConfiguration{Enabled: &on}
	helmYAML := `configVersion: 1
telemetryEnabled: true
profiling:
  enabled: true
  gatewayUiOtlpEndpoint: "10.0.0.50:4317"
`
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consts.OdigosConfigurationName,
			Namespace: consts.DefaultOdigosNamespace,
		},
		Data: map[string]string{
			consts.OdigosConfigurationFileName: helmYAML,
		},
	}
	c := fake.NewClientBuilder().WithObjects(cm).Build()
	got, err := ProfilingFromEffectiveOrHelm(context.Background(), c, effective)
	require.NoError(t, err)
	assert.Same(t, effective, got)
	assert.Equal(t, "10.0.0.50:4317", got.GatewayUiOtlpEndpoint)
}
