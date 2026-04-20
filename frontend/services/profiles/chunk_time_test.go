package profiles

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pprofile"
)

func TestEarliestProfileStartTimeUnixSec_DecodesBinaryChunk(t *testing.T) {
	pd := pprofile.NewProfiles()
	pd.Dictionary().StringTable().Append("cpu")
	prof := pd.ResourceProfiles().AppendEmpty().ScopeProfiles().AppendEmpty().Profiles().AppendEmpty()
	prof.SetProfileID([16]byte{1})
	// 1700000000000000000 ns = 1700000000 sec
	prof.SetTime(pcommon.Timestamp(1_700_000_000_000_000_000))

	m := &pprofile.ProtoMarshaler{}
	b, err := m.MarshalProfiles(pd)
	require.NoError(t, err)

	sec := earliestProfileStartTimeUnixSec([][]byte{b})
	assert.Equal(t, int64(1_700_000_000), sec)
}
