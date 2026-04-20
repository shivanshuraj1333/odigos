package profiles

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllNamesArePlaceholders_OnlyTotalIsFalse(t *testing.T) {
	assert.False(t, allNamesArePlaceholders([]string{"total"}))
	assert.False(t, allNamesArePlaceholders([]string{"total", "other"}))
}

func TestAllNamesArePlaceholders_SyntheticOnlyIsTrue(t *testing.T) {
	assert.True(t, allNamesArePlaceholders([]string{"total", "0xdeadbeef", "frame_12"}))
}

func TestAllNamesArePlaceholders_RealNameIsFalse(t *testing.T) {
	assert.False(t, allNamesArePlaceholders([]string{"total", "main", "0x1"}))
}
