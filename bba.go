package hbbft

import (
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

// AgreementMessage holds the epoch and the message sent in the BBA protocol.
type AgreementMessage struct {
	// Epoch when this message was sent.
	Epoch int
	// The actual contents of the message.
	Message interface{}
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

// CCRequest is not part of the HB protocol. We use it
// to interact with the CC asynchronously, to avoid blocking
// and to have a uniform interface from the user of the BBA.
type CCRequest struct {
	Payload interface{}
}

// DoneRequest is not part of the HB protocol, but we use
// it here to terminate the protocol in a graceful way.
type DoneRequest struct{}

// BBA is the Binary Byzantine Agreement build from a common coin protocol.
type BBA struct {
	// Config holds the BBA configuration.
	Config

	// Common Coin implementation to use.
	commonCoin      CommonCoin
	commonCoinAsked bool
	commonCoinValue *bool

	// Current epoch.
	epoch uint32

	// Bval requests we accepted this epoch (received from a quorum).
	binValues []bool

	// sentBvals are the binary values this instance sent in this epoch.
	sentBvals []bool

	// recvBval is a mapping of the sender and the received binary value.
	// We need to collect both values, because each node can send several
	// different bval messages in a single round.
	recvBvalT map[uint64]bool
	recvBvalF map[uint64]bool

	// recvAux is a mapping of the sender and the received Aux value.
	recvAux map[uint64]bool

	// recvDone contains received info, on which epoch which peer has decided.
	recvDone map[uint64]uint32

	// Whether this bba is terminated or not.
	// It can have an output, but should be still running,
	// because it has to help other peers to decide.
	done bool

	// output and estimated of the bba protocol.
	// This can be either nil or a boolean.
	// The decision is kept permanently, while the output is cleared on first fetch.
	output, estimated, decision interface{}

	//delayedMessages are messages that are received by a node that is already
	// in a later epoch. These messages will be queued an handled the next epoch.
	delayedMessages []*delayedMessage

	// For all the external access, like Done, Output, etc.
	lock sync.RWMutex

	// Queue of AgreementMessages that need to be broadcasted after each received
	// message.
	messages []*AgreementMessage

	// control flow tuples for internal channel communication.
	closeCh   chan struct{}
	inputCh   chan bbaInputTuple
	messageCh chan bbaMessageTuple
	msgCount  int
}

// NewBBA returns a new instance of the Binary Byzantine Agreement.
func NewBBA(cfg Config, nodeID uint64) *BBA {
	if cfg.F == -1 {
		cfg.F = (cfg.N - 1) / 3
	}
	var cc CommonCoin
	if cfg.CommonCoin != nil {
		cc = cfg.CommonCoin
	} else {
		cc = NewFakeCoin() // Use it by default to avoid breaking changes in the API.
	}
	bba := &BBA{
		Config:          cfg,
		commonCoin:      cc.ForNodeID(nodeID),
		commonCoinAsked: false,
		commonCoinValue: nil,
		recvBvalT:       make(map[uint64]bool),
		recvBvalF:       make(map[uint64]bool),
		recvAux:         make(map[uint64]bool),
		recvDone:        make(map[uint64]uint32),
		sentBvals:       []bool{},
		binValues:       []bool{},
		closeCh:         make(chan struct{}),
		inputCh:         make(chan bbaInputTuple),
		messageCh:       make(chan bbaMessageTuple),
		messages:        []*AgreementMessage{},
		delayedMessages: []*delayedMessage{},
	}
	go bba.run()
	return bba
}

// Control flow structure for internal channel communication. Allowing us to
// avoid the use of mutexes and eliminates race conditions.
type (
	bbaMessageTuple struct {
		senderID uint64
		msg      *AgreementMessage
		err      chan error
	}

	bbaInputTuple struct {
		value bool
		err   chan error
	}

	delayedMessage struct {
		sid uint64
		msg *AgreementMessage
	}
)

// InputValue will set the given val as the initial value to be proposed in the
// Agreement and returns an initial AgreementMessage or an error.
func (b *BBA) InputValue(val bool) error {
	t := bbaInputTuple{
		value: val,
		err:   make(chan error),
	}
	b.inputCh <- t
	return <-t.err
}

// HandleMessage will process the given rpc message. The caller is responsible to
// make sure only RPC messages are passed that are eligible for the BBA protocol.
func (b *BBA) HandleMessage(senderID uint64, msg *AgreementMessage) error {
	b.msgCount++
	t := bbaMessageTuple{
		senderID: senderID,
		msg:      msg,
		err:      make(chan error),
	}
	b.messageCh <- t
	return <-t.err
}

// AcceptInput returns true whether this bba instance is eligable for accepting
// a new input value.
func (b *BBA) AcceptInput() bool {
	return b.epoch == 0 && b.estimated == nil
}

// Output will return the output of the bba instance. If the output was not nil
// then it will return the output else nil. Note that after consuming the output
// its will be set to nil forever.
func (b *BBA) Output() interface{} {
	b.lock.Lock()
	defer b.lock.Unlock()
	if b.done && b.output != nil {
		out := b.output
		b.output = nil
		return out
	}
	return nil
}

// Messages returns the que of messages. The message que get's filled after
// processing a protocol message. After calling this method the que will
// be empty. Hence calling Messages can only occur once in a single roundtrip.
func (b *BBA) Messages() []*AgreementMessage {
	b.lock.Lock()
	defer b.lock.Unlock()
	msgs := b.messages
	b.messages = []*AgreementMessage{}
	return msgs
}

// addMessage adds single message to be broadcasted.
// The actual recipients are set in the ACS part.
func (b *BBA) addMessage(msg *AgreementMessage) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.messages = append(b.messages, msg)
}

// Done indicates, if the process is already decided and can be terminated.
// It can be so that the BBA has decided, but needs to support other peers to decide.
func (b *BBA) Done() bool {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.done
}

// Stop the BBA thread.
func (b *BBA) Stop() {
	close(b.closeCh)
}

// run makes sure we only process 1 message at the same time, avoiding mutexes
// and race conditions.
func (b *BBA) run() {
	for {
		select {
		case <-b.closeCh:
			return
		case t, ok := <-b.inputCh:
			if ok {
				t.err <- b.inputValue(t.value)
			}
		case t, ok := <-b.messageCh:
			if ok {
				t.err <- b.handleMessage(t.senderID, t.msg)
			}
		}
	}
}

// inputValue will set the given val as the initial value to be proposed in the
// Agreement.
func (b *BBA) inputValue(val bool) error {
	// Make sure we are in the first epoch and the value is not estimated yet.
	if b.epoch != 0 || b.estimated != nil {
		return nil
	}
	b.estimated = val
	return b.sendBval(val)
}

// handleMessage will process the given rpc message. The caller is responsible to
// make sure only RPC messages are passed that are eligible for the BBA protocol.
func (b *BBA) handleMessage(senderID uint64, msg *AgreementMessage) error {
	if _, ok := msg.Message.(*DoneRequest); ok {
		// We handle done messages from all the epochs.
		return b.handleDoneRequest(senderID, uint32(msg.Epoch))
	}
	if b.done {
		return nil
	}
	// Ignore messages from older epochs.
	if msg.Epoch < int(b.epoch) {
		log.Debugf(
			"id (%d) with epoch (%d) received msg from an older epoch (%d)",
			b.ID, b.epoch, msg.Epoch,
		)
		return nil
	}
	// Messages from later epochs will be queued and processed later.
	if msg.Epoch > int(b.epoch) {
		b.delayedMessages = append(b.delayedMessages, &delayedMessage{senderID, msg})
		return nil
	}

	switch t := msg.Message.(type) {
	case *BvalRequest:
		return b.handleBvalRequest(senderID, t.Value)
	case *AuxRequest:
		return b.handleAuxRequest(senderID, t.Value)
	case *CCRequest:
		return b.handleCCRequest(t.Payload)
	default:
		return fmt.Errorf("unknown BBA message received: %v", t)
	}
}

// handleBvalRequest processes the received binary value and fills up the
// message que if there are any messages that need to be broadcasted.
func (b *BBA) handleBvalRequest(senderID uint64, val bool) error {
	if b.commonCoinAsked {
		return nil
	}
	b.addRecvBval(senderID, val)
	lenBval := b.countRecvBvals(val)

	// When receiving n bval(b) messages from 2f+1 nodes: inputs := inputs u {b}
	// The set binValues can increase over time (more that a single element).
	if lenBval > 2*b.F {
		wasEmptyBinValues := len(b.binValues) == 0
		b.addBinValue(val)
		// If inputs > 0 broadcast output(b) and handle the output ourselfs.
		// Wait until binValues > 0, then broadcast AUX(b). The AUX(b) broadcast
		// may only occur once per epoch.
		if wasEmptyBinValues {
			b.addMessage(NewAgreementMessage(int(b.epoch), &AuxRequest{val}))
			if err := b.handleAuxRequest(b.ID, val); err != nil {
				return err
			}
		}
	}
	// When receiving input(b) messages from f + 1 nodes, if inputs(b) is not
	// been sent yet broadcast input(b) and handle the input ourselfs.
	if lenBval > b.F && !b.hasSentBval(val) {
		return b.sendBval(val)
	}

	// It is possible that we have the needed aux messages already,
	// therefore we need to try to make a decision.
	return b.tryOutputAgreement()
}

func (b *BBA) handleAuxRequest(senderID uint64, val bool) error {
	if b.commonCoinAsked {
		return nil
	}
	if _, ok := b.recvAux[senderID]; ok {
		// Only a single aux can be received from a peer.
		return fmt.Errorf("aux already received, recvNode=%v, epoch=%v, aux=%+v, new %v->%v", b.ID, b.epoch, b.recvAux, senderID, val)
	}
	b.recvAux[senderID] = val
	return b.tryOutputAgreement()
}

func (b *BBA) handleCCRequest(payload interface{}) error {
	coin, outPayloads, err := b.commonCoin.HandleRequest(b.epoch, payload)
	if err != nil {
		return err
	}
	if err := b.sendCC(outPayloads); err != nil {
		return err
	}
	if b.commonCoinValue == nil {
		b.commonCoinValue = coin
	}
	return b.tryOutputAgreement()
}

func (b *BBA) handleDoneRequest(senderID uint64, doneInEpoch uint32) error {
	b.recvDone[senderID] = doneInEpoch
	if b.canMarkDoneNow() {
		b.done = true
	}
	return nil
}

// tryOutputAgreement waits until at least (N - f) output messages received,
// once the (N - f) messages are received, make a common coin and uses it to
// compute the next decision estimate and output the optional decision value.
func (b *BBA) tryOutputAgreement() error {
	if len(b.binValues) == 0 {
		return nil
	}
	// Wait longer till eventually receive (N - F) aux messages.
	lenOutputs, values := b.countGoodAux()
	if lenOutputs < b.N-b.F {
		return nil
	}

	if !b.commonCoinAsked {
		maybeCoin, ccPayloads, err := b.commonCoin.StartCoinFlip(b.epoch)
		if err != nil {
			return err
		}
		if err := b.sendCC(ccPayloads); err != nil {
			return err
		}
		b.commonCoinAsked = true
		if b.commonCoinValue == nil {
			b.commonCoinValue = maybeCoin
		}
	}
	if b.commonCoinValue == nil {
		// Still waiting for the common coin.
		return nil
	}
	coin := *b.commonCoinValue

	// Continue the BBA until both:
	// - a value b is output in some epoch r
	// - the value (coin r) = b for some round r' > r
	if b.done || (b.decision != nil && b.decision.(bool) == coin) {
		b.done = true
		return nil
	}

	log.Debugf(
		"id (%d) is advancing to the next epoch! (%d) received (%d) aux messages",
		b.ID, b.epoch+1, lenOutputs,
	)

	// Start the next epoch.
	b.advanceEpoch()

	if len(values) == 1 {
		b.estimated = values[0]
		if b.decision == nil && values[0] == coin { // Output may be set only once.
			b.output = values[0]
			b.decision = values[0]
			log.Debugf("id (%d) outputed a decision (%v) after (%d) msgs", b.ID, values[0], b.msgCount)
			b.msgCount = 0
			if err := b.sendDone(); err != nil {
				return err
			}
		}
	} else {
		b.estimated = coin
	}
	if err := b.sendBval(b.estimated.(bool)); err != nil {
		return err
	}

	// Handle the delayed messages. They can be re-added to the delayed list,
	// if their epoch is still in the future (in the handleMessage function).
	deliveryCandidates := b.delayedMessages
	b.delayedMessages = []*delayedMessage{}
	for _, delayed := range deliveryCandidates {
		if err := b.handleMessage(delayed.sid, delayed.msg); err != nil {
			return err
		}
	}
	return nil
}

func (b *BBA) sendBval(val bool) error {
	b.sentBvals = append(b.sentBvals, val)
	b.addMessage(NewAgreementMessage(int(b.epoch), &BvalRequest{val}))
	return b.handleBvalRequest(b.ID, val)
}

func (b *BBA) sendCC(outPayloads []interface{}) error {
	for _, outPayload := range outPayloads {
		b.addMessage(NewAgreementMessage(int(b.epoch), &CCRequest{Payload: outPayload}))
	}
	return nil
}

func (b *BBA) sendDone() error {
	if _, ok := b.recvDone[b.ID]; ok {
		return nil
	}
	b.addMessage(NewAgreementMessage(int(b.epoch), &DoneRequest{}))
	return b.handleDoneRequest(b.ID, b.epoch)
}

// advanceEpoch will reset all the values that are bound to an epoch and increments
// the epoch value by 1.
func (b *BBA) advanceEpoch() {
	b.binValues = []bool{}
	b.sentBvals = []bool{}
	b.recvAux = make(map[uint64]bool)
	b.recvBvalT = make(map[uint64]bool)
	b.recvBvalF = make(map[uint64]bool)
	b.commonCoinAsked = false
	b.commonCoinValue = nil
	b.epoch++
}

// countGoodAux returns the number of received (aux) messages, the corresponding
// values that where also in our inputs.
func (b *BBA) countGoodAux() (int, []bool) {
	// Collect aux messages, that were received with values also present in binValues.
	goodAux := map[uint64]bool{}
	for i := range b.recvAux {
		for _, binVal := range b.binValues {
			if b.recvAux[i] == binVal {
				goodAux[i] = binVal
			}
		}
	}
	// Take only the values present in the goodAux
	values := []bool{}
	for _, auxVal := range goodAux {
		found := false
		for _, val := range values {
			if auxVal == val {
				found = true
				break
			}
		}
		if !found {
			values = append(values, auxVal)
		}
	}
	return len(goodAux), values
}

// addRecvBval marks the specified BVAL_r(val) as received.
// The same node can send multiple values, we track them separatelly.
func (b *BBA) addRecvBval(senderID uint64, val bool) {
	if val {
		b.recvBvalT[senderID] = val
	} else {
		b.recvBvalF[senderID] = val
	}
}

// countRecvBvals counts all the received Bval inputs matching b.
func (b *BBA) countRecvBvals(val bool) int {
	if val {
		return len(b.recvBvalT)
	} else {
		return len(b.recvBvalF)
	}
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

func (b *BBA) addBinValue(val bool) {
	for _, bv := range b.binValues {
		if bv == val {
			return
		}
	}
	b.binValues = append(b.binValues, val)
}

// If others (more than F) have decided in previous epochs, then we are
// among the others, who decided in a subsequent round, therefore we don't
// need to wait for more epochs to close the process.
func (b *BBA) canMarkDoneNow() bool {
	if _, ok := b.recvDone[b.ID]; !ok {
		// We have not decided yet, can't close the process.
		return false
	}
	count := 0
	for _, e := range b.recvDone {
		if e < b.recvDone[b.ID] {
			count++
		}
	}
	return count > b.F
}
