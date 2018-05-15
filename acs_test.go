package hbbft

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewACS(t *testing.T) {
	nodes := []uint64{1, 2, 3}
	acs := NewACS(Config{
		N:  4,
		ID: 4,
	}, nodes)
	assert.Equal(t, 4, len(acs.bbaInstances))
	assert.Equal(t, 4, len(acs.rbcInstances))

	for i := range acs.rbcInstances {
		_, ok := acs.bbaInstances[i]
		assert.True(t, ok)
	}

	for i := range acs.bbaInstances {
		_, ok := acs.bbaInstances[i]
		assert.True(t, ok)
	}
}
