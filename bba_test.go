package hbbft

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBBA(t *testing.T) {
	cfg := Config{N: 4}
	bba := NewBBA(cfg)
	assert.Equal(t, 0, len(bba.binValues))
	assert.Equal(t, 0, len(bba.recvBval))
	assert.Equal(t, 0, len(bba.recvAux))
	assert.Equal(t, 0, len(bba.sentBvals))
	assert.Equal(t, uint32(0), bba.epoch)
	assert.Equal(t, false, bba.done)
	assert.Nil(t, bba.output)
}

func TestAdvanceEpochInBBA(t *testing.T) {
	cfg := Config{N: 4}
	bba := NewBBA(cfg)
	bba.epoch = 8
	bba.binValues = []bool{false, true, true}
	bba.sentBvals = []bool{false, true}
	bba.recvAux = map[uint64]bool{
		1:    false,
		3949: true,
	}
	bba.advanceEpoch()
	assert.Equal(t, 0, len(bba.recvAux))
	assert.Equal(t, 0, len(bba.sentBvals))
	assert.Equal(t, 0, len(bba.binValues))
	assert.Equal(t, uint32(8+1), bba.epoch)
}

func TestHandleBvalRequest(t *testing.T) {
	cfg := Config{N: 4}
	bba := NewBBA(cfg)

	// Expected output is nil, we only expect an output:
	// inputs(b) 2 * (F + 1)
	out, err := bba.handleBvalRequest(1, true)
	assert.Nil(t, err)
	assert.Nil(t, out)

	// Expected output should have
	bba.recvBval = map[uint64]bool{
		1: true,
		2: true,
		3: true,
	}
	out, err = bba.handleBvalRequest(0, true)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(out.Messages))
	assert.IsType(t, &AuxRequest{}, out.Messages[0].Message)
}
