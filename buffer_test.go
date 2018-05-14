package hbbft

import (
	"encoding/binary"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBufferPush(t *testing.T) {
	var (
		b    = newBuffer()
		n    = 100
		data = make([]tx, n)
	)
	for i := 0; i < n; i++ {
		v := tx{rand.Uint64()}
		data[i] = v
		b.push(v)
	}
	assert.Equal(t, n, b.len())
	for i := 0; i < n; i++ {
		assert.Equal(t, data[i], b.data[i])
	}
}

func TestBufferSample(t *testing.T) {
	var (
		b    = newBuffer()
		n    = 1000
		data = make([]tx, n)
	)
	for i := 0; i < n; i++ {
		v := tx{uint64(i)}
		data[i] = v
		b.push(v)
	}
	txx := b.sample(10, 50)
	assert.Equal(t, 10, len(txx))

	// must be in range 0 - 50.
	for _, v := range txx {
		if v.(tx).Value() > uint64(50) {
			t.Fatal("err")
		}
	}
}

type tx struct {
	Nonce uint64
}

func (t tx) Encode(w io.Writer) error {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, t.Value())
	_, err := w.Write(buf)
	return err
}

func (t tx) Value() uint64 { return t.Nonce }
