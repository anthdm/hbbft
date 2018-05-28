package hbbft

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test ACS with 4 good nodes. The result should be that at least the output
// of (N - f) nodes has been provided.
func TestACSGoodNodes(t *testing.T) {
	inputs := map[int][]byte{
		0: []byte("AAAAAA"),
		1: []byte("BBBBBB"),
		2: []byte("CCCCCC"),
		3: []byte("DDDDDD"),
	}
	testCommonSubset(t, inputs)
}

func testCommonSubset(t *testing.T, inputs map[int][]byte) {
	type acsResult struct {
		nodeID  uint64
		results map[uint64][]byte
	}
	var (
		resultCh = make(chan acsResult, 4)
		nodes    = makeACSNetwork(4)
		messages = make(chan testMsg)
	)

	go func() {
		for {
			select {
			case msg := <-messages:
				acs := nodes[msg.msg.To]
				err := acs.HandleMessage(msg.from, msg.msg.Payload.(*ACSMessage))
				if err != nil {
					t.Fatal(err)
				}
				for _, msg := range acs.messageQue.messages() {
					go func(msg MessageTuple) {
						messages <- testMsg{acs.ID, msg}
					}(msg)
				}
				if output := acs.Output(); output != nil {
					resultCh <- acsResult{acs.ID, output}
				}
			}
		}
	}()

	for nodeID, value := range inputs {
		assert.Nil(t, nodes[nodeID].InputValue(value))
		for _, msg := range nodes[nodeID].messageQue.messages() {
			messages <- testMsg{uint64(nodeID), msg}
			time.Sleep(1 * time.Millisecond)
		}
	}

	count := 0
	for res := range resultCh {
		assert.True(t, len(res.results) >= len(nodes)-1)
		for id, result := range res.results {
			assert.Equal(t, inputs[int(id)], result)
		}
		count++
		if count == 4 {
			break
		}
	}
}

func TestNewACS(t *testing.T) {
	var (
		id    = uint64(0)
		nodes = []uint64{0, 1, 2, 3}
		acs   = NewACS(Config{
			N:     len(nodes),
			ID:    id,
			Nodes: nodes,
		})
	)
	assert.Equal(t, len(nodes), len(acs.bbaInstances))
	assert.Equal(t, len(nodes), len(acs.rbcInstances))

	for i := range acs.rbcInstances {
		_, ok := acs.bbaInstances[i]
		assert.True(t, ok)
	}
	for i := range acs.bbaInstances {
		_, ok := acs.bbaInstances[i]
		assert.True(t, ok)
	}
	assert.Equal(t, id, acs.ID)
}

func TestACSOutputIsNilAfterConsuming(t *testing.T) {
	acs := NewACS(Config{N: 4})
	output := map[uint64][]byte{
		1: []byte("this is it"),
	}
	acs.output = output
	assert.Equal(t, output, acs.Output())
	assert.Nil(t, acs.Output())
}

type testMsg struct {
	from uint64
	msg  MessageTuple
}

func makeACSNetwork(n int) []*ACS {
	network := make([]*ACS, n)
	for i := 0; i < n; i++ {
		network[i] = NewACS(Config{N: n, ID: uint64(i), Nodes: makeids(n)})
		go network[i].run()
	}
	return network
}

func makeids(n int) []uint64 {
	ids := make([]uint64, n)
	for i := 0; i < n; i++ {
		ids[i] = uint64(i)
	}
	return ids
}

// makeTransports is a test helper function for making n number of transports.
func makeTransports(n int) []Transport {
	transports := make([]Transport, n)
	for i := 0; i < n; i++ {
		transports[i] = NewLocalTransport(uint64(i))
	}
	return transports
}
