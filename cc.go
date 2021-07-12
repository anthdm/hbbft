package hbbft

// CommonCoin is an interface to be provided by the real CC implementation.
type CommonCoin interface {
	ForNodeID(nodeID uint64) CommonCoin
	HandleRequest(epoch uint32, payload interface{}) (*bool, []interface{}, error)
	StartCoinFlip(epoch uint32) (*bool, []interface{}, error)
}

// fakeCoin is a trivial incorrect implementation of the common coin interface.
// It is used here just to avoid dependencies to particular crypto libraries.
type fakeCoin struct{}

// Let's return the same coin for 2 times in a row.
// That can lead to faster consensus, because the next round is successful
// when the coin is the same as the first one with some decisions.
// It is beneficial to start such a sequence with [true, true, ...].
func NewFakeCoin() CommonCoin {
	return &fakeCoin{}
}

func (fc *fakeCoin) ForNodeID(nodeID uint64) CommonCoin {
	return &fakeCoin{}
}
func (fc *fakeCoin) HandleRequest(epoch uint32, payload interface{}) (*bool, []interface{}, error) {
	panic("HandleRequest is not used in this implementation")
}
func (fc *fakeCoin) StartCoinFlip(epoch uint32) (*bool, []interface{}, error) {
	coin := (epoch/2)%2 == 0
	return &coin, []interface{}{}, nil
}
