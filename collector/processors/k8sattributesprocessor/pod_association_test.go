// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package k8sattributesprocessor

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/client"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/k8sattributesprocessor/internal/kube"
)

func TestExtractPodIDSkipsNonIPHostNameAssociation(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("host.name", "k8s-node-1")

	associations := []kube.Association{
		{
			Sources: []kube.AssociationSource{{
				From: kube.ResourceSource,
				Name: "host.name",
			}},
		},
	}

	pid := extractPodID(t.Context(), attrs, associations)
	assert.False(t, pid.IsNotEmpty())
}

func TestExtractPodIDFallsBackWhenHostNameIsNotIP(t *testing.T) {
	ctx := client.NewContext(t.Context(), client.Info{
		Addr: &net.TCPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 4317},
	})
	attrs := pcommon.NewMap()
	attrs.PutStr("host.name", "worker-node")

	associations := []kube.Association{
		{
			Sources: []kube.AssociationSource{{
				From: kube.ResourceSource,
				Name: "host.name",
			}},
		},
		{
			Sources: []kube.AssociationSource{{
				From: kube.ConnectionSource,
			}},
		},
	}

	pid := extractPodID(ctx, attrs, associations)
	require.True(t, pid.IsNotEmpty())
	assert.Equal(t, kube.ConnectionSource, pid[0].Source.From)
	assert.Equal(t, "1.2.3.4", pid[0].Value)
}

func TestExtractPodIDKeepsHostNameWhenValueIsIP(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("host.name", "10.1.2.3")

	associations := []kube.Association{
		{
			Sources: []kube.AssociationSource{{
				From: kube.ResourceSource,
				Name: "host.name",
			}},
		},
	}

	pid := extractPodID(t.Context(), attrs, associations)
	require.True(t, pid.IsNotEmpty())
	assert.Equal(t, kube.ResourceSource, pid[0].Source.From)
	assert.Equal(t, "host.name", pid[0].Source.Name)
	assert.Equal(t, "10.1.2.3", pid[0].Value)
}

func TestExtractPodIDNormalizesContainerIDAssociation(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("container.id", "containerd://abcdef123456")

	associations := []kube.Association{
		{
			Sources: []kube.AssociationSource{{
				From: kube.ResourceSource,
				Name: "container.id",
			}},
		},
	}

	pid := extractPodID(t.Context(), attrs, associations)
	require.True(t, pid.IsNotEmpty())
	assert.Equal(t, kube.ResourceSource, pid[0].Source.From)
	assert.Equal(t, "container.id", pid[0].Source.Name)
	assert.Equal(t, "abcdef123456", pid[0].Value)
}

func TestExtractPodIDCandidatesReturnsAllResolvedAssociations(t *testing.T) {
	ctx := client.NewContext(t.Context(), client.Info{
		Addr: &net.TCPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 4317},
	})
	attrs := pcommon.NewMap()
	attrs.PutStr("container.id", "containerd://abcdef123456")

	associations := []kube.Association{
		{
			Sources: []kube.AssociationSource{{
				From: kube.ResourceSource,
				Name: "container.id",
			}},
		},
		{
			Sources: []kube.AssociationSource{{
				From: kube.ConnectionSource,
			}},
		},
	}

	pids := extractPodIDCandidates(ctx, attrs, associations)
	require.Len(t, pids, 2)
	assert.Equal(t, "abcdef123456", pids[0][0].Value)
	assert.Equal(t, "1.2.3.4", pids[1][0].Value)
}
