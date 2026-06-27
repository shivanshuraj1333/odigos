package profiles

import (
	"testing"

	"github.com/odigos-io/odigos/frontend/services/profiles/flamegraph"
)

// TestPyroscopeMetadataFor_UnitsPerType asserts the flamebearer metadata units/name are correct for
// every selectable profile type: bytes for *_space, objects for *_objects, all under "memory"; cpu
// keeps samples/cpu.
func TestPyroscopeMetadataFor_UnitsPerType(t *testing.T) {
	for _, tc := range []struct {
		profileType string
		wantUnits   string
		wantName    string
	}{
		{flamegraph.SampleTypeCPU, pyroscopeMetadataUnitsSamples, pyroscopeMetadataProfileNameCPU},
		{flamegraph.SampleTypeAllocSpace, pyroscopeMetadataUnitsBytes, pyroscopeMetadataProfileNameMemory},
		{flamegraph.SampleTypeInuseSpace, pyroscopeMetadataUnitsBytes, pyroscopeMetadataProfileNameMemory},
		{flamegraph.SampleTypeAllocObjects, pyroscopeMetadataUnitsObjects, pyroscopeMetadataProfileNameMemory},
		{flamegraph.SampleTypeInuseObjects, pyroscopeMetadataUnitsObjects, pyroscopeMetadataProfileNameMemory},
	} {
		md := pyroscopeMetadataFor(tc.profileType)
		if string(md.Units) != tc.wantUnits {
			t.Errorf("%s: units=%q want %q", tc.profileType, md.Units, tc.wantUnits)
		}
		if md.Name != tc.wantName {
			t.Errorf("%s: name=%q want %q", tc.profileType, md.Name, tc.wantName)
		}
	}
}
