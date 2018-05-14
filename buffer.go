package hbbft

import "math/rand"

// buffer holds an uncommited pool of arbitrary data. In blockchain parlance
// this would be the unbound transaction list.
type buffer struct {
	data []Transaction
}

// newBuffer return a new initialized buffer with the max data cap set to 1024.
func newBuffer() *buffer {
	return &buffer{
		data: make([]Transaction, 0, 1024),
	}
}

// push pushes v to the buffer.
func (b *buffer) push(tx Transaction) {
	b.data = append(b.data, tx)
}

// len return the current length of the buffer.
func (b *buffer) len() int {
	return len(b.data)
}

// sample will return (n) elements from the buffer with (m) as its maximum
// upperbound.
// [ a ]
// [ b ]
// [ c ] m
// [ d ]
// In the above example there can be only sampled between a, b and c.
// If the length of the buffer is smaller then 1 no transactions are being
// returned. If the length of the buffer is smaller then (m) the total length of
// the buffer will be used as upperbound.
func (b *buffer) sample(n, m int) []Transaction {
	txx := []Transaction{}
	for i := 0; i < n; i++ {
		if b.len() <= 1 {
			break
		}
		if b.len() < m {
			m = b.len() - 1
		}
		index := rand.Intn(m)
		txx = append(txx, b.data[index])
		b.data = append(b.data[:index], b.data[index+1:]...)
	}
	return txx
}
