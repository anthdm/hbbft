package hbbft

// CommonCoin is an interface to be provided by the real CC implementation.
type CommonCoin interface {
	FlipCoin(epoch uint32) bool
}

// fakeCoin is a trivial incorrect implementation of the common coin interface.
// It is used here just to avoid dependencies to particular crypto libraries.
type fakeCoin struct{}

func NewFakeCoin() CommonCoin {
	return &fakeCoin{}
}

// Let's return the same coin for 2 times in a row.
// That can lead to faster consensus, because the next round is successful
// when the coin is the same as the first one with some decisions.
// It is beneficial to start such a sequence with [true, true, ...].
func (fc *fakeCoin) FlipCoin(epoch uint32) bool {
	return (epoch/2)%2 == 0
}
