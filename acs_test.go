package hbbft

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			F:     -1, // Use default.
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
	acs := NewACS(Config{N: 4, F: -1}) // Use default for F.
	output := map[uint64][]byte{
		1: []byte("this is it"),
	}
	acs.output = output
	assert.Equal(t, output, acs.Output())
	assert.Nil(t, acs.Output())
}

// This test checks, if messages sent in any order are still handled correctly.
// It is expected to run this test multiple times.
func TestACSRandomized(t *testing.T) {
	if err := testACSRandomized(t); err != nil {
		t.Fatalf("Failed, reason=%+v", err)
	}
}
func testACSRandomized(t *testing.T) error {
	var err error
	var N, T = 7, 5

	msgs := make([]*testMsg, 0)
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

	acs := make([]*ACS, N)
	for a := range acs {
		acs[a] = NewACS(cfg[a])
		if err = acs[a].InputValue([]byte{1, 2, byte(a)}); err != nil {
			return fmt.Errorf("Failed to process ACS.InputValue: %+v", err)
		}
		msgs = appendTestMsgs(acs[a].Messages(), nodes[a], msgs)
	}

	// var done bool
	for len(msgs) != 0 {
		m := rand.Intn(len(msgs))
		msg := msgs[m]

		msgTo := msg.msg.To
		if acsMsg, ok := msg.msg.Payload.(*ACSMessage); ok {
			if err = acs[msgTo].HandleMessage(uint64(msg.from), acsMsg); err != nil {
				return fmt.Errorf("Failed to ACS.HandleMessage: %+v", err)
			}
		} else {
			return fmt.Errorf("Unexpected message type: %+v", msg.msg.Payload)
		}

		// Remove the message from the buffer and append the new messages, if any.
		msgs[m] = msgs[len(msgs)-1]
		msgs = msgs[:len(msgs)-1]
		msgs = appendTestMsgs(acs[msgTo].Messages(), nodes[msgTo], msgs)
	}

	out0 := acs[0].Output()
	for a := range acs {
		require.True(t, acs[a].Done())
		if a == 0 {
			continue
		}
		var outA map[uint64][]byte = acs[a].Output()
		require.Equal(t, len(out0), len(outA))
		for i := range out0 {
			require.Equal(t, bytes.Compare(out0[i], outA[i]), 0)
		}
		acs[a].Stop()
	}
	return nil
}

type testMsg struct {
	from uint64
	msg  MessageTuple
}

func appendTestMsgs(msgs []MessageTuple, senderID uint64, buf []*testMsg) []*testMsg {
	output := buf[:]
	for m := range msgs {
		output = append(output, &testMsg{from: senderID, msg: msgs[m]})
	}
	return output
}

func makeACSNetwork(n int) []*ACS {
	network := make([]*ACS, n)
	for i := 0; i < n; i++ {
		network[i] = NewACS(Config{N: n, F: -1, ID: uint64(i), Nodes: makeids(n)}) // Use default for F.
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
