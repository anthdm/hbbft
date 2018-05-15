package hbbft

import (
	"errors"
	"fmt"
)

// AgreementMessage holds the epoch and the message sent in the BBA protocol.
type AgreementMessage struct {
	Epoch   int
	Message interface{}
}

// AgreementOutput holds the output after processing a BBA message.
type AgreementOutput struct {
	// Decision is the value which the agreement has decided. This can be either
	// a bool or nil.
	Decision interface{}
	// Messages holds a slice of agreement Messages that need to be broadcasted after
	// processing the message. This makes testing the messages transport independent.
	// And allows returned messages to be propagated to a top level protocol.
	Messages []*AgreementMessage
}

// AddMessage only adds the given msg if its not nil.
func (o *AgreementOutput) AddMessage(msg *AgreementMessage) {
	if msg != nil {
		o.Messages = append(o.Messages, msg)
	}
}

// NewAgreementOutput constructs a new AgreementOutput.
func NewAgreementOutput() *AgreementOutput {
	return &AgreementOutput{
		Messages: []*AgreementMessage{},
	}
}

// NewAgreementMessage constructs a new AgreementMessage.
func NewAgreementMessage(e int, msg interface{}) *AgreementMessage {
	return &AgreementMessage{
		Epoch:   e,
		Message: msg,
	}
}

// BvalRequest holds the input value of the binary input.
type BvalRequest struct {
	Value bool
}

// AuxRequest holds the output value.
type AuxRequest struct {
	Value bool
}

// BBA is the Binary Byzantine Agreement build from a common coin protocol.
type BBA struct {
	// Config holds the BBA configuration.
	Config

	// Current epoch.
	epoch uint32

	binValues []bool

	// sentBvals are the binary values this instance sended.
	sentBvals []bool

	// recvBval is a mapping of the sender and the receveived binary value.
	recvBval map[uint64]bool

	// recvAux is a mapping of the sender and the receveived Aux value.
	recvAux map[uint64]bool

	estimated, done bool
	output          interface{}
}

// NewBBA returns a new instance of the Binary Byzantine Agreement.
func NewBBA(cfg Config) *BBA {
	if cfg.F == 0 {
		cfg.F = (cfg.N - 1) / 3
	}
	return &BBA{
		Config:    cfg,
		recvBval:  make(map[uint64]bool),
		recvAux:   make(map[uint64]bool),
		sentBvals: []bool{},
		binValues: []bool{},
	}
}

// Propose will set the given val as the initial value to be proposed in the
// Agreement.
func (b *BBA) Propose(val bool) error {
	// Make sure we are in the first epoch round.
	if b.epoch != 0 {
		return errors.New("proposing initial value can only be done in the first epoch")
	}
	b.estimated = val
	// Handle the value internally.
	b.recvBval[b.ID] = val
	msg := NewAgreementMessage(int(b.epoch), &BvalRequest{val})
	go b.Transport.Broadcast(b.ID, msg)
	return nil
}

// HandleMessage will process the given rpc message. The caller is resposible to
// make sure only RPC messages are passed that are elligible for the BBA protocol.
func (b *BBA) HandleMessage(senderID uint64, msg AgreementMessage) (*AgreementOutput, error) {
	// Make sure we only handle messages that are sent in the same epoch.
	if msg.Epoch != int(b.epoch) {
		return nil, fmt.Errorf("received msg from other epoch: %d", msg.Epoch)
	}
	if b.done {
		return nil, errors.New("bba already terminated")
	}
	switch t := msg.Message.(type) {
	case *BvalRequest:
		return b.handleBvalRequest(senderID, t.Value)
	case *AuxRequest:
		return b.handleAuxRequest(senderID, t.Value)
	default:
		return nil, fmt.Errorf("unknown BBA message received: %v", t)
	}
}

func (b *BBA) handleBvalRequest(senderID uint64, val bool) (*AgreementOutput, error) {
	output := NewAgreementOutput()
	b.recvBval[senderID] = val
	lenIn := b.countBvals(val)

	// When receiving Input(b) messages from 2f+1 nodes:
	// inputs := inputs u {b}
	if lenIn == 2*(b.F+1) {
		b.binValues = append(b.binValues, val)
		// If inputs == 1 broadcast output(b) and handle the output ourselfs.
		if len(b.binValues) == 1 {
			msg := NewAgreementMessage(int(b.epoch), &AuxRequest{val})
			output.AddMessage(msg)
			b.recvAux[b.ID] = val
		}
		decision, msg := b.maybeOutputAgreement()
		output.AddMessage(msg)
		output.Decision = decision
		return output, nil
	}
	// When receiving input(b) messages from f + 1 nodes, if inputs(b) is not
	// been sent yet broadcast input(b) and handle the input ourselfs.
	if lenIn == b.F+1 && !b.hasSentBval(val) {
		msg := NewAgreementMessage(int(b.epoch), &BvalRequest{val})
		output.AddMessage(msg)
		b.recvBval[b.ID] = val
	}
	return nil, nil
}

func (b *BBA) handleAuxRequest(senderID uint64, val bool) (*AgreementOutput, error) {
	b.recvAux[senderID] = val
	if len(b.binValues) > 0 {
		decision, msg := b.maybeOutputAgreement()
		return &AgreementOutput{
			Decision: decision,
			Messages: []*AgreementMessage{msg},
		}, nil
	}
	return nil, nil
}

// maybeOutputAgreement waits until at least (N - f) output messages received,
// once the (N - f) messages are received, make a common coin and uses it to
// compute the next decision estimate and output the optional decision value.
func (b *BBA) maybeOutputAgreement() (interface{}, *AgreementMessage) {
	lenOutputs, outputs := b.countOutputs()
	if lenOutputs < b.N-b.F {
		return nil, nil
	}

	coin := b.epoch%2 == 0

	// continue the BBA until both:
	// - a value b is output in some epoch r
	// - the value (coin r) = b for some round r' > r
	if b.output != nil && b.output.(bool) == coin {
		b.done = true
	}

	// Start the next epoch.
	b.advanceEpoch()

	var decision interface{}
	if len(outputs) != 1 {
		b.estimated = coin
		decision = nil
	} else {
		b.estimated = outputs[0]
		// Output may be set only once.
		if b.output == nil && outputs[0] == coin {
			b.output = coin
			decision = b.output
		}
	}
	msg := NewAgreementMessage(int(b.epoch), &BvalRequest{b.estimated})
	return decision, msg
}

// advanceEpoch will reset all the values that are bound to an epoch and increments
// the epoch value by 1.
func (b *BBA) advanceEpoch() {
	b.binValues = []bool{}
	b.recvAux = make(map[uint64]bool)
	b.epoch++
}

// countOutputs returns the number of received outputs that are also in our inputs.
// and return the mathing values.
func (b *BBA) countOutputs() (int, []bool) {
	var (
		n    = 0
		vals = []bool{}
	)
	for _, val := range b.recvAux {
		for _, ok := range b.binValues {
			if ok == val {
				n++
				vals = append(vals, val)
			}
		}
	}
	return n, vals
}

// countBvals counts all the received Bval inputs matching b.
func (b *BBA) countBvals(ok bool) int {
	n := 0
	for _, val := range b.recvBval {
		if val == ok {
			n++
		}
	}
	return n
}

// hasSentBval return true if we already sent out the given value.
func (b *BBA) hasSentBval(val bool) bool {
	for _, ok := range b.sentBvals {
		if ok == val {
			return true
		}
	}
	return false
}
