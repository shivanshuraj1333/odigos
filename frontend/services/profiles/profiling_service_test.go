package profiles

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClearProfilingBufferForSource_NoSlot(t *testing.T) {
	s := NewProfileStore(4, 3600, 1024, time.Hour)
	_, err := ClearProfilingBufferForSource(s, "default", "Deployment", "noslot")
	require.Error(t, err)
}

func TestClearProfilingBufferForSource_OK(t *testing.T) {
	s := NewProfileStore(4, 3600, 1024, time.Hour)
	_, err := EnableProfilingForSource(s, "default", "Deployment", "svc")
	require.NoError(t, err)
	s.AddProfileData("default/Deployment/svc", []byte("x"))
	out, err := ClearProfilingBufferForSource(s, "default", "Deployment", "svc")
	require.NoError(t, err)
	require.Equal(t, "ok", out.Status)
	require.Equal(t, "default/Deployment/svc", out.SourceKey)
	require.Empty(t, s.GetProfileData("default/Deployment/svc"))
	require.True(t, s.IsActive("default/Deployment/svc"))
}
