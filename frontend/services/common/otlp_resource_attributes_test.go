package common

import (
	"testing"

	"github.com/odigos-io/odigos/api/k8sconsts"
	odigosconsts "github.com/odigos-io/odigos/common/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func TestResourceAttributesToSourceID_OdigosKindLowercaseMatchesCanonical(t *testing.T) {
	m := pcommon.NewMap()
	m.PutStr(string(semconv.K8SNamespaceNameKey), "default")
	m.PutStr(odigosconsts.OdigosWorkloadKindAttribute, "deployment")
	m.PutStr(odigosconsts.OdigosWorkloadNameAttribute, "shop")

	sid, err := ResourceAttributesToSourceID(m)
	require.NoError(t, err)
	assert.Equal(t, "default", sid.Namespace)
	assert.Equal(t, k8sconsts.WorkloadKindDeployment, sid.Kind)
	assert.Equal(t, "shop", sid.Name)
}

func TestResourceAttributesToSourceID_OdigosKindLowercaseResolvesK8sDeploymentName(t *testing.T) {
	m := pcommon.NewMap()
	m.PutStr(string(semconv.K8SNamespaceNameKey), "prod")
	m.PutStr(odigosconsts.OdigosWorkloadKindAttribute, "deployment")
	m.PutStr(string(semconv.K8SDeploymentNameKey), "api")

	sid, err := ResourceAttributesToSourceID(m)
	require.NoError(t, err)
	assert.Equal(t, k8sconsts.WorkloadKindDeployment, sid.Kind)
	assert.Equal(t, "api", sid.Name)
}

func TestResourceAttributesToSourceID_RolloutAlias(t *testing.T) {
	m := pcommon.NewMap()
	m.PutStr(string(semconv.K8SNamespaceNameKey), "ns1")
	m.PutStr(odigosconsts.OdigosWorkloadKindAttribute, "rollout")
	m.PutStr(odigosconsts.OdigosWorkloadNameAttribute, "svc")

	sid, err := ResourceAttributesToSourceID(m)
	require.NoError(t, err)
	assert.Equal(t, k8sconsts.WorkloadKindArgoRollout, sid.Kind)
}
