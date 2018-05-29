package hbbft

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Testing BBA should cover all of the following specifications.
//
// 1. If a correct node outputs the value (b), then every good node outputs (b).
// 2. If all good nodes receive input, then every good node outputs a value.
// 3. If any good node ouputs value (b), then at least one good ndoe receives (b)
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
	bba := NewBBA(Config{N: 4, ID: 0})

	// Set our input value.
	assert.Nil(t, bba.InputValue(true))
	assert.Equal(t, 1, len(bba.sentBvals))
	assert.True(t, bba.sentBvals[0])
	assert.True(t, bba.recvBval[0]) // we are id (0)
	msgs := bba.Messages()
	assert.Equal(t, 1, len(msgs))
	assert.IsType(t, &BvalRequest{}, msgs[0].Message)
	assert.True(t, msgs[0].Message.(*BvalRequest).Value)

	// Sent input from node 1
	bba.handleBvalRequest(uint64(1), true)
	assert.True(t, bba.recvBval[1])

	// Sent input from node 2
	// The algorithm decribes that after receiving (N - f) bval messages we
	// broadcast AUX(b)
	bba.handleBvalRequest(uint64(2), true)
	assert.True(t, bba.recvBval[2])
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
	cfg := Config{N: 4}
	bba := NewBBA(cfg)
	assert.Equal(t, 0, len(bba.binValues))
	assert.Equal(t, 0, len(bba.recvBval))
	assert.Equal(t, 0, len(bba.recvAux))
	assert.Equal(t, 0, len(bba.sentBvals))
	assert.Equal(t, uint32(0), bba.epoch)
	assert.Equal(t, false, bba.done)
	assert.Nil(t, bba.output)
}

func TestAdvanceEpochInBBA(t *testing.T) {
	cfg := Config{N: 4}
	bba := NewBBA(cfg)
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
		bbas[i] = NewBBA(Config{N: n, ID: uint64(i)})
		go bbas[i].run()
	}
	return bbas
}

type testAgreementMessage struct {
	from uint64
	to   uint64
	msg  *AgreementMessage
}
