package hbbft

import (
	"fmt"
)

// ACSOutput holds the output of the Common Subset protocol.
type ACSOutput struct {
	Messages []interface{}
	Result   interface{}
}

// ACSMessage represents a message sent between nodes in the ACS protocol.
type ACSMessage struct {
	NodeID  uint64
	Payload interface{}
}

// ACS implements the Asynchronous Common Subset protocol.
// ACS assumes a network of N nodes that send signed messages to each other.
// There can be f faulty nodes where (3 * f < N).
// Each participating node proposes an element for inlcusion. The protocol
// guarantees that all of the good nodes output the same set, consisting of
// at least (N -f) of the proposed values.
//
// Algorithm:
// ACS creates a Broadcast algorithm for each of the participating nodes.
// At least (N -f) of these will eventually output the element proposed by that
// node. ACS will also create and BBA instance for each participating node, to
// decide whether that node's proposed element should be inlcuded in common set.
// Whenever an element is received via broadcast, we imput "true" into the
// corresponding BBA instance. When (N-f) BBA instances have decided true we
// input false into the remaining ones, where we haven't provided input yet.
// Once all BBA instances have decided, ACS returns the set of all proposed
// values for which the decision was truthy.
type ACS struct {
	// Config holds the ACS configuration.
	Config
	// Mapping of node ids and their rbc instance.
	rbcInstances map[uint64]*RBC
	// Mapping of node ids and their bba instance.
	bbaInstances map[uint64]*BBA
	// Results of the Reliable Broadcast.
	rbcResults map[uint64][]byte
	// Results of the Binary Byzantine Agreement.
	bbaResults map[uint64]bool
}

// NewACS returns a new ACS instance configured with the given Config and node
// ids.
func NewACS(cfg Config, nodes []uint64) *ACS {
	acs := &ACS{
		Config:       cfg,
		rbcInstances: make(map[uint64]*RBC),
		bbaInstances: make(map[uint64]*BBA),
		rbcResults:   make(map[uint64][]byte),
		bbaResults:   make(map[uint64]bool),
	}
	// Add ourself the participating nodes.
	nodes = append(nodes, cfg.ID)
	// Create all the instances for the participating nodes
	for _, id := range nodes {
		acs.rbcInstances[id] = NewRBC(cfg, id)
		acs.bbaInstances[id] = NewBBA(cfg)
	}
	return acs
}

// InputValue sets the input value for broadcast and returns a slice of messages
// that need to be broadcasted into the network.
func (a *ACS) InputValue(data []byte) ([]*ACSMessage, []interface{}, error) {
	rbc, ok := a.rbcInstances[a.ID]
	if !ok {
		return nil, nil, fmt.Errorf("could not find our rbc instance %d", a.ID)
	}
	msgs, err := rbc.InputValue(data)
	if err != nil {
		return nil, nil, err
	}
	acsMsgs := make([]*ACSMessage, len(msgs))
	for i, msg := range msgs {
		acsMsgs[i] = &ACSMessage{a.ID, msg}
	}
	return acsMsgs, nil, nil
}

// HandleMessage handles incoming messages to ACS and redirects them to the
// appropriate sub(protocol) instance.
func (a *ACS) HandleMessage(senderID uint64, msg *ACSMessage) (*ACSOutput, error) {
	switch t := msg.Payload.(type) {
	case *AgreementMessage:
		return a.handleAgreement(senderID, msg.NodeID, t)
	case *BroadcastMessage:
		return a.handleBroadcast(senderID, msg.NodeID, t)
	default:
		return nil, fmt.Errorf("received unknown message: %v", t)
	}
}

// handleBroadcast processes the given BroadcastMessage and redirects it to the
// proper RBC instance.
func (a *ACS) handleBroadcast(senderID, proposerID uint64, msg *BroadcastMessage) (*ACSOutput, error) {
	rbc, ok := a.rbcInstances[proposerID]
	if !ok {
		return nil, fmt.Errorf("could not find the rbc instance for node %d", a.ID)
	}
	output, err := rbc.HandleMessage(senderID, msg)
	if err != nil {
		return nil, err
	}

	messages := []interface{}{}
	// If we have an rbc output.
	if output != nil {
		msg, err := a.handleBroadcastResult(a.ID, output)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	msgs := rbc.Messages()
	for _, msg := range msgs {
		messages = append(messages, msg)
	}
	out := &ACSOutput{
		Messages: messages,
	}
	return out, nil
}

// handleAgreement processes the given AgreementMessage and redirects it to the
// proper BBA instance.
func (a *ACS) handleAgreement(senderID, proposerID uint64, msg *AgreementMessage) (*ACSOutput, error) {
	bba, ok := a.bbaInstances[proposerID]
	if !ok {
		return nil, fmt.Errorf("could not find the bba instance for node %d", a.ID)
	}
	output, err := bba.HandleMessage(senderID, *msg)
	if err != nil {
		return nil, err
	}
	if output == nil {
		return nil, nil
	}

	messages := []interface{}{}
	if output.Decision != nil {
		msgs, err := a.handleAgreementResult(proposerID, output.Decision.(bool))
		if err != nil {
			return nil, err
		}
		for _, msg := range msgs {
			if msg != nil {
				messages = append(messages, msg)
			}
		}
	}
	for _, msg := range output.Messages {
		if msg != nil {
			messages = append(messages, msg)
		}
	}
	return &ACSOutput{
		Messages: messages,
		Result:   a.maybeCompleteAgreement(),
	}, nil
}

func (a *ACS) handleAgreementResult(proposerID uint64, result bool) ([]*AgreementMessage, error) {
	a.bbaResults[proposerID] = result
	if !result || a.countTruthyAgreements() < a.N-a.F {
		return nil, nil
	}
	// When receiving true from at least N - f nodes, provide input 0 to each
	// bba instance that has not provided input yet.
	messages := []*AgreementMessage{}
	for _, bba := range a.bbaInstances {
		if bba.AcceptInput() {
			msg, err := bba.InputValue(false)
			if err != nil {
				return nil, err
			}
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

// handleBroadcastResult is triggered when there is an ouput value of the rbc
// protocol. And start the bba protocol accordingly for this rbc outcome.
// Upon delivery of v from RBC, if input has not yet been provided to BBA, then
// provide input 1 to BBA.
func (a *ACS) handleBroadcastResult(proposerID uint64, result []byte) (*AgreementMessage, error) {
	a.rbcResults[proposerID] = result
	bba, ok := a.bbaInstances[proposerID]
	if !ok {
		return nil, fmt.Errorf("could not find the bba instance for id %d", proposerID)
	}
	return bba.InputValue(true)
}

// maybeCompleteAgreement checks if all the instances of bba are terminated.
func (a *ACS) maybeCompleteAgreement() [][]byte {
	// Check if all the bba instances are done.
	for _, bba := range a.bbaInstances {
		if !bba.done {
			return nil
		}
	}
	// Collect all the id's of nodes that have decided true.
	ids := []uint64{}
	for id, ok := range a.bbaResults {
		if ok {
			ids = append(ids, id)
		}
	}
	// TODO ..
	return nil
}

// countTruthyAgreements returns the number of truthy received agreement messages.
func (a *ACS) countTruthyAgreements() int {
	n := 0
	for _, ok := range a.bbaResults {
		if ok {
			n++
		}
	}
	return n
}
