package services

import (
	"context"
	"os"
	"testing"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/frontend/kube"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/yaml"
)

const testNamespace = "test-ns"

// setupFakeKubeClient replaces kube.DefaultClient with a fake Kubernetes client.
// The previous value is restored when the test finishes.
func setupFakeKubeClient(t *testing.T) *k8sfake.Clientset {
	t.Helper()
	fakeCS := k8sfake.NewSimpleClientset()
	prev := kube.DefaultClient
	kube.DefaultClient = &kube.Client{Interface: fakeCS}
	t.Cleanup(func() { kube.DefaultClient = prev })
	return fakeCS
}

// setTestNamespace sets CURRENT_NS via t.Setenv (automatically restored after test).
func setTestNamespace(t *testing.T, ns string) {
	t.Helper()
	t.Setenv(consts.CurrentNamespaceEnvVar, ns)
}

// createOwnerConfigMap creates the odigos-configuration configmap that is required as an
// owner reference when SetComponentLogLevel creates odigos-local-ui-config for the first time.
func createOwnerConfigMap(t *testing.T, fakeCS *k8sfake.Clientset, ns string) types.UID {
	t.Helper()
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consts.OdigosConfigurationName,
			Namespace: ns,
			UID:       "owner-uid-1234",
		},
	}
	_, err := fakeCS.CoreV1().ConfigMaps(ns).Create(context.Background(), cm, metav1.CreateOptions{})
	require.NoError(t, err)
	return cm.UID
}

// ---------------------------------------------------------------------------
// Tests for setComponentLogLevelField (pure function, no k8s deps)
// ---------------------------------------------------------------------------

func TestSetComponentLogLevelField_AllComponents(t *testing.T) {
	tests := []struct {
		component string
		level     common.OdigosLogLevel
		check     func(*common.ComponentLogLevels) common.OdigosLogLevel
	}{
		{
			component: "autoscaler",
			level:     common.LogLevelDebug,
			check:     func(c *common.ComponentLogLevels) common.OdigosLogLevel { return c.Autoscaler },
		},
		{
			component: "scheduler",
			level:     common.LogLevelWarn,
			check:     func(c *common.ComponentLogLevels) common.OdigosLogLevel { return c.Scheduler },
		},
		{
			component: "instrumentor",
			level:     common.LogLevelError,
			check:     func(c *common.ComponentLogLevels) common.OdigosLogLevel { return c.Instrumentor },
		},
		{
			component: "odiglet",
			level:     common.LogLevelDebug,
			check:     func(c *common.ComponentLogLevels) common.OdigosLogLevel { return c.Odiglet },
		},
		{
			component: "deviceplugin",
			level:     common.LogLevelInfo,
			check:     func(c *common.ComponentLogLevels) common.OdigosLogLevel { return c.Deviceplugin },
		},
		{
			component: "ui",
			level:     common.LogLevelWarn,
			check:     func(c *common.ComponentLogLevels) common.OdigosLogLevel { return c.UI },
		},
		{
			component: "collector",
			level:     common.LogLevelError,
			check:     func(c *common.ComponentLogLevels) common.OdigosLogLevel { return c.Collector },
		},
		{
			// empty string → default field
			component: "",
			level:     common.LogLevelDebug,
			check:     func(c *common.ComponentLogLevels) common.OdigosLogLevel { return c.Default },
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run("component="+tc.component, func(t *testing.T) {
			c := &common.ComponentLogLevels{}
			setComponentLogLevelField(c, tc.component, tc.level)
			got := tc.check(c)
			assert.Equal(t, tc.level, got,
				"expected field for component %q to be %q, got %q", tc.component, tc.level, got)
		})
	}
}

func TestSetComponentLogLevelField_NilReceiver(t *testing.T) {
	// Should not panic when receiver is nil.
	require.NotPanics(t, func() {
		setComponentLogLevelField(nil, "autoscaler", common.LogLevelDebug)
	})
}

func TestSetComponentLogLevelField_OtherFieldsUntouched(t *testing.T) {
	// Setting one field must not clobber others.
	c := &common.ComponentLogLevels{
		Default:    common.LogLevelWarn,
		Autoscaler: common.LogLevelError,
	}
	setComponentLogLevelField(c, "scheduler", common.LogLevelDebug)
	assert.Equal(t, common.LogLevelWarn, c.Default, "Default should be unchanged")
	assert.Equal(t, common.LogLevelError, c.Autoscaler, "Autoscaler should be unchanged")
	assert.Equal(t, common.LogLevelDebug, c.Scheduler, "Scheduler should be set")
}

// ---------------------------------------------------------------------------
// Tests for SetComponentLogLevel (uses kube.DefaultClient)
// ---------------------------------------------------------------------------

// Test: configmap does not exist → function creates it with correct content.
func TestSetComponentLogLevel_CreateWhenAbsent(t *testing.T) {
	setTestNamespace(t, testNamespace)
	fakeCS := setupFakeKubeClient(t)
	ownerUID := createOwnerConfigMap(t, fakeCS, testNamespace)

	ctx := context.Background()
	err := SetComponentLogLevel(ctx, nil, "autoscaler", common.LogLevelDebug)
	require.NoError(t, err)

	// Verify the configmap was created.
	cm, err := fakeCS.CoreV1().ConfigMaps(testNamespace).Get(ctx, consts.OdigosLocalUiConfigName, metav1.GetOptions{})
	require.NoError(t, err)

	// Verify owner reference points to odigos-configuration.
	require.Len(t, cm.OwnerReferences, 1)
	assert.Equal(t, ownerUID, cm.OwnerReferences[0].UID)
	assert.Equal(t, consts.OdigosConfigurationName, cm.OwnerReferences[0].Name)

	// Verify data content.
	rawYAML, ok := cm.Data[consts.OdigosConfigurationFileName]
	require.True(t, ok, "expected config.yaml key in configmap data")

	var cfg common.OdigosConfiguration
	require.NoError(t, yaml.Unmarshal([]byte(rawYAML), &cfg))
	require.NotNil(t, cfg.ComponentLogLevels)
	assert.Equal(t, common.LogLevelDebug, cfg.ComponentLogLevels.Autoscaler)
}

// Test: configmap exists → updates only the targeted component, leaves others intact.
func TestSetComponentLogLevel_UpdateExisting(t *testing.T) {
	setTestNamespace(t, testNamespace)
	fakeCS := setupFakeKubeClient(t)
	createOwnerConfigMap(t, fakeCS, testNamespace)

	// Pre-populate odigos-local-ui-config with an existing config.
	existing := common.OdigosConfiguration{
		ComponentLogLevels: &common.ComponentLogLevels{
			Default:   common.LogLevelWarn,
			Scheduler: common.LogLevelError,
		},
	}
	existingData, err := yaml.Marshal(existing)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = fakeCS.CoreV1().ConfigMaps(testNamespace).Create(ctx, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consts.OdigosLocalUiConfigName,
			Namespace: testNamespace,
		},
		Data: map[string]string{consts.OdigosConfigurationFileName: string(existingData)},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Now update instrumentor only.
	err = SetComponentLogLevel(ctx, nil, "instrumentor", common.LogLevelDebug)
	require.NoError(t, err)

	// Read back and verify.
	cm, err := fakeCS.CoreV1().ConfigMaps(testNamespace).Get(ctx, consts.OdigosLocalUiConfigName, metav1.GetOptions{})
	require.NoError(t, err)

	var cfg common.OdigosConfiguration
	require.NoError(t, yaml.Unmarshal([]byte(cm.Data[consts.OdigosConfigurationFileName]), &cfg))
	require.NotNil(t, cfg.ComponentLogLevels)
	assert.Equal(t, common.LogLevelDebug, cfg.ComponentLogLevels.Instrumentor, "instrumentor must be updated")
	assert.Equal(t, common.LogLevelWarn, cfg.ComponentLogLevels.Default, "default must be preserved")
	assert.Equal(t, common.LogLevelError, cfg.ComponentLogLevels.Scheduler, "scheduler must be preserved")
}

// Test: concurrent create race → AlreadyExists on Create → retries → succeeds as update.
//
// We use a PrependReactor on the fake client: the first call to Create for the local-ui-config
// configmap injects the object into the tracker and then returns AlreadyExists. This causes
// SetComponentLogLevel to wrap the error as a Conflict and retry the outer loop. On the retry
// the Get succeeds (the object is now in the tracker) and the function proceeds to Update.
func TestSetComponentLogLevel_ConcurrentCreateRace(t *testing.T) {
	setTestNamespace(t, testNamespace)
	fakeCS := setupFakeKubeClient(t)
	createOwnerConfigMap(t, fakeCS, testNamespace)

	ctx := context.Background()
	createCallCount := 0

	fakeCS.PrependReactor("create", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction, ok := action.(k8stesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		obj := createAction.GetObject()
		configMap, ok := obj.(*v1.ConfigMap)
		if !ok || configMap.Name != consts.OdigosLocalUiConfigName {
			// Pass through for the owner configmap and any other creates.
			return false, nil, nil
		}
		createCallCount++
		if createCallCount == 1 {
			// Simulate race: add the object to the tracker so the retry Get succeeds,
			// then return AlreadyExists to trigger the conflict-retry path.
			addErr := fakeCS.Tracker().Add(configMap)
			require.NoError(t, addErr, "tracker.Add must not fail")
			gr := schema.GroupResource{Group: "", Resource: "configmaps"}
			return true, nil, apierrors.NewAlreadyExists(gr, consts.OdigosLocalUiConfigName)
		}
		// Should not be called a second time — Update path is taken on retry.
		return false, nil, nil
	})

	err := SetComponentLogLevel(ctx, nil, "odiglet", common.LogLevelWarn)
	require.NoError(t, err)

	// The Create reactor was only triggered once; Update handled the rest.
	assert.Equal(t, 1, createCallCount, "Create should be called exactly once before the race is detected")

	// Verify the configmap contains the expected level.
	cm, err := fakeCS.CoreV1().ConfigMaps(testNamespace).Get(ctx, consts.OdigosLocalUiConfigName, metav1.GetOptions{})
	require.NoError(t, err)

	var cfg common.OdigosConfiguration
	require.NoError(t, yaml.Unmarshal([]byte(cm.Data[consts.OdigosConfigurationFileName]), &cfg))
	require.NotNil(t, cfg.ComponentLogLevels)
	assert.Equal(t, common.LogLevelWarn, cfg.ComponentLogLevels.Odiglet)
}

// Test: CURRENT_NS unset → falls back to "odigos-system".
func TestSetComponentLogLevel_DefaultNamespaceFallback(t *testing.T) {
	// Ensure CURRENT_NS is unset → falls back to consts.DefaultOdigosNamespace.
	os.Unsetenv(consts.CurrentNamespaceEnvVar) //nolint:errcheck
	fakeCS := setupFakeKubeClient(t)
	createOwnerConfigMap(t, fakeCS, consts.DefaultOdigosNamespace)

	err := SetComponentLogLevel(context.Background(), nil, "scheduler", common.LogLevelInfo)
	require.NoError(t, err)

	cm, err := fakeCS.CoreV1().ConfigMaps(consts.DefaultOdigosNamespace).Get(
		context.Background(), consts.OdigosLocalUiConfigName, metav1.GetOptions{})
	require.NoError(t, err)

	var cfg common.OdigosConfiguration
	require.NoError(t, yaml.Unmarshal([]byte(cm.Data[consts.OdigosConfigurationFileName]), &cfg))
	require.NotNil(t, cfg.ComponentLogLevels)
	assert.Equal(t, common.LogLevelInfo, cfg.ComponentLogLevels.Scheduler)
}
