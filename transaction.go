package hbbft

import "io"

// Transaction is an interface that abstract the underlying data of the actual
// transaction. This allows package hbbft to be easily adopted by other
// applications.
type Transaction interface {
	Encode(io.Writer) error
	Decode(io.Reader) error
	Hash() []byte
}
