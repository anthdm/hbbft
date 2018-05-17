package hbbft

import (
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"
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
	//  Bval requests we accepted this epoch.
	binValues []bool
	// sentBvals are the binary values this instance sent.
	sentBvals []bool
	// recvBval is a mapping of the sender and the receveived binary value.
	recvBval map[uint64]bool
	// recvAux is a mapping of the sender and the receveived Aux value.
	recvAux map[uint64]bool
	// Whether this bba is terminated or not.
	done bool
	// output and estimated of the bba protocol. This can be either nil or a
	// boolean.
	output, estimated interface{}
	// control flow tuples for internal channel communication.
	inputCh   chan inputTuple
	messageCh chan messageTuple
}

// NewBBA returns a new instance of the Binary Byzantine Agreement.
func NewBBA(cfg Config) *BBA {
	if cfg.F == 0 {
		cfg.F = (cfg.N - 1) / 3
	}
	bba := &BBA{
		Config:    cfg,
		recvBval:  make(map[uint64]bool),
		recvAux:   make(map[uint64]bool),
		sentBvals: []bool{},
		binValues: []bool{},
		inputCh:   make(chan inputTuple),
		messageCh: make(chan messageTuple),
	}
	go bba.run()
	return bba
}

// Control flow structure for internal channel communication. Allowing us to
// avoid the use of mutexes and eliminates race conditions.
type (
	messageTuple struct {
		senderID uint64
		msg      AgreementMessage
		response chan messageResponse
	}

	messageResponse struct {
		output *AgreementOutput
		err    error
	}

	inputResponse struct {
		msg *AgreementMessage
		err error
	}

	inputTuple struct {
		value    bool
		response chan inputResponse
	}
)

// InputValue will set the given val as the initial value to be proposed in the
// Agreement and returns an initial AgreementMessage or an error.
func (b *BBA) InputValue(val bool) (*AgreementMessage, error) {
	t := inputTuple{
		value:    val,
		response: make(chan inputResponse),
	}
	b.inputCh <- t
	resp := <-t.response
	return resp.msg, resp.err
}

// HandleMessage will process the given rpc message. The caller is resposible to
// make sure only RPC messages are passed that are elligible for the BBA protocol.
func (b *BBA) HandleMessage(senderID uint64, msg AgreementMessage) (*AgreementOutput, error) {
	t := messageTuple{
		senderID: senderID,
		msg:      msg,
		response: make(chan messageResponse),
	}
	b.messageCh <- t
	resp := <-t.response
	return resp.output, resp.err
}

// AcceptInput returns true whether this bba instance is elligable for accepting
// a new input value.
func (b *BBA) AcceptInput() bool {
	return b.epoch == 0 && b.estimated != nil
}

// run makes sure we only process 1 message at the same time, avoiding mutexes
// and race conditions.
func (b *BBA) run() {
	for {
		select {
		case input := <-b.inputCh:
			msg, err := b.inputValue(input.value)
			input.response <- inputResponse{msg, err}
		case t := <-b.messageCh:
			out, err := b.handleMessage(t.senderID, t.msg)
			t.response <- messageResponse{out, err}
		}
	}
}

// inputValue will set the given val as the initial value to be proposed in the
// Agreement and returns an initial AgreementMessage or an error.
func (b *BBA) inputValue(val bool) (*AgreementMessage, error) {
	// Make sure we are in the first epoch round.
	if b.epoch != 0 {
		return nil, errors.New(
			"proposing initial value can only be done in the first epoch")
	}
	b.estimated = val
	// Set the value as sent internally.
	b.recvBval[b.ID] = val
	msg := NewAgreementMessage(int(b.epoch), &BvalRequest{val})
	return msg, nil
}

// handleMessage will process the given rpc message. The caller is resposible to
// make sure only RPC messages are passed that are elligible for the BBA protocol.
func (b *BBA) handleMessage(senderID uint64, msg AgreementMessage) (*AgreementOutput, error) {
	// Make sure we only handle messages that are sent in the same epoch.
	if msg.Epoch != int(b.epoch) {
		log.Warnf("received msg from other epoch %d my epoch %d", msg.Epoch, b.epoch)
		return nil, nil
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

// handleBvalRequest processes the received binary value and returns an
// AgreementOutput.
func (b *BBA) handleBvalRequest(senderID uint64, val bool) (*AgreementOutput, error) {
	output := NewAgreementOutput()
	b.recvBval[senderID] = val
	lenBval := b.countBvals(val)

	// When receiving n bval(b) messages from 2f+1 nodes: inputs := inputs u {b}
	if lenBval == 2*(b.F+1) {
		wasEmptyBinValues := len(b.binValues) == 0
		b.binValues = append(b.binValues, val)
		// If inputs == 1 broadcast output(b) and handle the output ourselfs.
		// Wait until binValues > 0, then broadcast AUX(b). The AUX(b) broadcast
		// may only occure once per epoch.
		if wasEmptyBinValues {
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
	if lenBval == b.F+1 && !b.hasSentBval(val) {
		b.sentBvals = append(b.sentBvals, val)
		b.recvBval[b.ID] = val
		msg := NewAgreementMessage(int(b.epoch), &BvalRequest{val})
		output.AddMessage(msg)
	}
	return output, nil
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
	// Wait longer till eventually receive (N - F) aux messages.
	lenOutputs, values := b.countOutputs()
	if lenOutputs < b.N-b.F {
		return nil, nil
	}

	coin := b.epoch%2 == 0

	// Continue the BBA until both:
	// - a value b is output in some epoch r
	// - the value (coin r) = b for some round r' > r
	if b.output != nil && b.output.(bool) == coin {
		b.done = true
	}

	log.Infof("advancing to the nex epoch! I have received %d aux messages", lenOutputs)

	// Start the next epoch.
	b.advanceEpoch()

	var decision interface{}
	if len(values) < 1 {
		b.estimated = coin
		decision = nil
	} else {
		b.estimated = values[0]
		// Output may be set only once.
		if b.output == nil && values[0] == coin {
			b.output = coin
			decision = b.output
		}
	}
	estimated := b.estimated.(bool)
	b.sentBvals = append(b.sentBvals, estimated)
	msg := NewAgreementMessage(int(b.epoch), &BvalRequest{estimated})
	return decision, msg
}

// advanceEpoch will reset all the values that are bound to an epoch and increments
// the epoch value by 1.
func (b *BBA) advanceEpoch() {
	b.binValues = []bool{}
	b.sentBvals = []bool{}
	b.recvAux = make(map[uint64]bool)
	b.recvBval = make(map[uint64]bool)
	b.epoch++
}

// countOutputs returns the number of received (aux) messages, the corresponding
// values that where also in our inputs.
func (b *BBA) countOutputs() (int, []bool) {
	vals := []bool{}
	for _, ok := range b.binValues {
		for _, val := range b.recvAux {
			if ok == val {
				vals = append(vals, val)
				break
			}
		}
	}
	return len(b.recvAux), vals
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
