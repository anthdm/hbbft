package hbbft

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Testing BBA should cover all of the following specifications.
//
// 1. If a correct node outputs the value (b), then every good node outputs (b).
// 2. If all good nodes receive input, then every good node outputs a value.
// 3. If any good node outputs value (b), then at least one good node receives (b)
// as input.

// func TestAllNodesFaultyAgreement(t *testing.T) {
// 	logrus.SetLevel(logrus.DebugLevel)
// 	testAgreement(t, []bool{false, false, false, false}, false)
// }

func TestFaultyAgreement(t *testing.T) {
	testAgreement(t, []bool{true, false, false, false}, false)
}

// Test BBA with 2 false and 2 true nodes, cause binary agreement is not a
// majority vote it guarantees that all good nodes output a least the output of
// one good node. Hence the output should be true for all the nodes.
func TestAgreement2FalseNodes(t *testing.T) {
	testAgreement(t, []bool{true, false, true, false}, true)
}

func TestAgreement1FalseNode(t *testing.T) {
	testAgreement(t, []bool{true, false, true, true}, true)
}

func TestAgreementGoodNodes(t *testing.T) {
	testAgreement(t, []bool{true, true, true, true}, true)
}

func TestBBAStepByStep(t *testing.T) {
	bba := NewBBA(Config{N: 4, F: -1, ID: 0}, 0)

	// Set our input value.
	assert.Nil(t, bba.InputValue(true))
	assert.Equal(t, 1, len(bba.sentBvals))
	assert.True(t, bba.sentBvals[0])
	assert.True(t, bba.recvBvalT[0]) // we are id (0)
	msgs := bba.Messages()
	assert.Equal(t, 1, len(msgs))
	assert.IsType(t, &BvalRequest{}, msgs[0].Message)
	assert.True(t, msgs[0].Message.(*BvalRequest).Value)

	// Sent input from node 1
	bba.handleBvalRequest(uint64(1), true)
	assert.True(t, bba.recvBvalT[1])

	// Sent input from node 2
	// The algorithm decribes that after receiving (N - f) bval messages we
	// broadcast AUX(b)
	bba.handleBvalRequest(uint64(2), true)
	assert.True(t, bba.recvBvalT[2])
	msg := bba.Messages()
	assert.Equal(t, 1, len(msg))
	assert.IsType(t, &AuxRequest{}, msg[0].Message)
	assert.True(t, msg[0].Message.(*AuxRequest).Value)
	assert.True(t, bba.recvAux[0]) // our id

	// Let's assume node 1 and node 2 are good nodes and also sent their AUX
	// message
	bba.handleAuxRequest(uint64(1), true)
	assert.True(t, bba.recvAux[1])

	// If now node 2 sents his AUX(true) we should advance to the next epoch and
	// have a decision.
	bba.handleAuxRequest(uint64(2), true)
	assert.Equal(t, true, bba.output.(bool))
	assert.Equal(t, true, bba.decision.(bool))
	assert.Equal(t, uint32(1), bba.epoch)
}

func TestNewBBA(t *testing.T) {
	cfg := Config{N: 4, F: -1}
	bba := NewBBA(cfg, 0)
	assert.Equal(t, 0, len(bba.binValues))
	assert.Equal(t, 0, len(bba.recvBvalT))
	assert.Equal(t, 0, len(bba.recvBvalF))
	assert.Equal(t, 0, len(bba.recvAux))
	assert.Equal(t, 0, len(bba.sentBvals))
	assert.Equal(t, uint32(0), bba.epoch)
	assert.Equal(t, false, bba.done)
	assert.Nil(t, bba.output)
}

func TestAdvanceEpochInBBA(t *testing.T) {
	cfg := Config{N: 4, F: -1}
	bba := NewBBA(cfg, 0)
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

func testAgreement(t *testing.T, inputs []bool, expect bool) {
	assert.True(t, len(inputs) == 4)
	var (
		messages = make(chan testAgreementMessage)
		bbas     = makeBBAInstances(4)
		result   = make(chan bool, 4)
	)
	go func() {
		for {
			select {
			case msg := <-messages:
				bba := bbas[msg.to]
				if err := bba.HandleMessage(msg.from, msg.msg); err != nil {
					t.Fatal(err)
				}
				for _, msg := range bba.Messages() {
					for _, id := range excludeID([]uint64{0, 1, 2, 3}, bba.ID) {
						go func(msg *AgreementMessage, id uint64) {
							messages <- testAgreementMessage{bba.ID, id, msg}
						}(msg, id)
					}
				}
				if output := bba.Output(); output != nil {
					result <- output.(bool)
				}
				for _, msg := range bba.Messages() {
					for _, id := range excludeID([]uint64{0, 1, 2, 3}, bba.ID) {
						go func(msg *AgreementMessage, id uint64) {
							messages <- testAgreementMessage{bba.ID, id, msg}
						}(msg, id)
					}
				}
				if output := bba.Output(); output != nil {
					result <- output.(bool)
				}
			}
		}
	}()

	for i, b := range inputs {
		assert.Nil(t, bbas[i].InputValue(b))
		for _, msg := range bbas[i].Messages() {
			for _, id := range excludeID([]uint64{0, 1, 2, 3}, bbas[i].ID) {
				messages <- testAgreementMessage{bbas[i].ID, id, msg}
			}
		}
	}

	counter := 0
	for res := range result {
		assert.Equal(t, expect, res)
		counter++
		if counter == 4 {
			break
		}
	}
}

func excludeID(ids []uint64, id uint64) []uint64 {
	dest := []uint64{}
	for _, i := range ids {
		if i != id {
			dest = append(dest, i)
		}
	}
	return dest
}

func makeBBAInstances(n int) []*BBA {
	bbas := make([]*BBA, n)
	for i := 0; i < n; i++ {
		bbas[i] = NewBBA(Config{N: n, F: -1, ID: uint64(i)}, uint64(i))
	}
	return bbas
}

type testAgreementMessage struct {
	from uint64
	to   uint64
	msg  *AgreementMessage
}

func appendTestAgreementMessages(msgs []*AgreementMessage, senderID uint64, nodes []uint64, buf []*testAgreementMessage) []*testAgreementMessage {
	output := buf[:]
	for n := range nodes {
		if nodes[n] != senderID {
			for m := range msgs {
				output = append(output, &testAgreementMessage{from: senderID, to: nodes[n], msg: msgs[m]})
			}
		}
	}
	return output
}

// This test should be run repeatedly for some time.
func TestBBARandomized(t *testing.T) {
	if err := testBBARandomized(t); err != nil {
		t.Fatalf("Failed, reason=%+v", err)
	}
}
func testBBARandomized(t *testing.T) error {
	var err error
	var N, T = 7, 5

	msgs := make([]*testAgreementMessage, 0)
	nodes := make([]uint64, N)
	for n := range nodes {
		nodes[n] = uint64(n)
	}

	cfg := make([]Config, N)
	for i := range cfg {
		cfg[i] = Config{
			N:         N,
			F:         N - T,
			ID:        uint64(i),
			Nodes:     nodes,
			BatchSize: 21254, // Should be unused.
		}
	}

	inputs := make([]bool, N)
	for i := range inputs {
		inputs[i] = rand.Int()%2 == 0
	}

	bba := make([]*BBA, N)
	for i := range bba {
		bba[i] = NewBBA(cfg[i], nodes[i])
		if err = bba[i].InputValue(inputs[i]); err != nil {
			return fmt.Errorf("Failed to process BBA.InputValue: %v", err)
		}
		msgs = appendTestAgreementMessages(bba[i].Messages(), nodes[i], nodes, msgs)
	}

	for len(msgs) != 0 {
		m := rand.Intn(len(msgs))
		msg := msgs[m]
		msgTo := msg.to
		if err = bba[msgTo].HandleMessage(msg.from, msg.msg); err != nil {
			return fmt.Errorf("Failed to BBA.HandleMessage: %v", err)
		}

		// Remove the message from the buffer and add the new messages.
		msgs[m] = msgs[len(msgs)-1]
		msgs = msgs[:len(msgs)-1]
		msgs = appendTestAgreementMessages(bba[msgTo].Messages(), nodes[msgTo], nodes, msgs)
	}

	out0 := bba[0].Output().(bool)
	for i := range bba {
		require.True(t, bba[i].done)
		if i != 0 {
			require.Equal(t, bba[i].Output(), out0)
		}
		bba[i].Stop()
	}
	return nil
}
