package hbbft

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"testing"

	"github.com/klauspost/reedsolomon"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test RBC where 1 node will not provide its value. We use 4 nodes that will
// tolerate 1 faulty node. The 3 good nodes should be able te reconstruct the
// proposed value just fine.
func TestRBC1FaultyNode(t *testing.T) {
	var (
		n          = 4
		pid        = 0
		resCh      = make(chan bcResult, 3)
		value      = []byte("not a normal looking payload")
		faultyNode = uint64(3)
	)
	nodes := makeRBCNodes(n, pid, resCh)

	var wg sync.WaitGroup
	wg.Add(n - 1)
	// Start a routine that will collect the results from all the good nodes.
	// In this case 3 results should be equal to the proposed value.
	go func() {
		for {
			res := <-resCh
			// Test that we really have a faulty node.
			assert.NotEqual(t, res.nodeID, faultyNode)
			assert.Equal(t, value, res.value)
			wg.Done()
		}
	}()

	// Faulty node will not provide its input.
	nodes[faultyNode].faulty = true
	assert.Nil(t, nodes[pid].inputValue(value))
	wg.Wait()
}

// Test RBC with 4 good nodes in the network. We expect all 4 nodes to output
// the proposed value.
func TestRBC4GoodNodes(t *testing.T) {
	var (
		n     = 4
		pid   = 0
		resCh = make(chan bcResult)
		value = []byte("not a normal looking payload")
	)
	nodes := makeRBCNodes(n, pid, resCh)

	var wg sync.WaitGroup
	wg.Add(n)
	// Start a routine that will collect the results from all the nodes. In this
	// case we expect all 4 nodes to ouput the proposed value.
	go func() {
		for {
			res := <-resCh
			assert.Equal(t, value, res.value)
			wg.Done()
		}
	}()

	assert.Nil(t, nodes[pid].inputValue(value))
	wg.Wait()
}

func TestRBCInputValue(t *testing.T) {
	rbc := NewRBC(Config{
		N: 4,
		F: -1,
	}, 0)
	reqs, err := rbc.InputValue([]byte("this is a test string"))
	assert.Nil(t, err)
	assert.Equal(t, rbc.N-1, len(reqs))
	assert.Equal(t, 1, len(rbc.Messages()))
	assert.Equal(t, 0, len(rbc.messages))
}

func TestNewReliableBroadcast(t *testing.T) {
	assertState := func(t *testing.T, rb *RBC, cfg Config) {
		assert.NotNil(t, rb.enc)
		assert.NotNil(t, rb.recvEchos)
		assert.NotNil(t, rb.recvReadys)
		assert.Equal(t, rb.numParityShards, cfg.F*2)
		assert.Equal(t, rb.numDataShards, cfg.N-rb.numParityShards)
		assert.Equal(t, 0, len(rb.messages))
	}

	cfg := Config{N: 4, F: 1}
	rb := NewRBC(cfg, 0)
	assertState(t, rb, cfg)

	cfg = Config{N: 18, F: 4}
	rb = NewRBC(cfg, 0)
	assertState(t, rb, cfg)

	cfg = Config{N: 100, F: 10}
	rb = NewRBC(cfg, 0)
	assertState(t, rb, cfg)
}

func TestRBCOutputIsNilAfterConsuming(t *testing.T) {
	rbc := NewRBC(Config{N: 4, F: -1}, 0)
	output := []byte("a")
	rbc.output = output
	assert.Equal(t, output, rbc.Output())
	assert.Nil(t, rbc.Output())
}

func TestRBCMessagesIsEmptyAfterConsuming(t *testing.T) {
	rbc := NewRBC(Config{N: 4, F: -1}, 0)
	rbc.messages = []*BroadcastMessage{{}}
	assert.Equal(t, 1, len(rbc.Messages()))
	assert.Equal(t, 0, len(rbc.Messages()))
}

func TestMakeShards(t *testing.T) {
	var (
		data = []byte("this is a very normal string.")
		nP   = 2
		nD   = 4
	)
	for i := 0; i < 10; i++ {
		nP += i
		nD += i
		enc, err := reedsolomon.New(nD, nP)
		assert.Nil(t, err)
		shards, err := makeShards(enc, data)
		assert.Nil(t, err)
		assert.Equal(t, nP+nD, len(shards))
	}
}

func TestMakeProofRequests(t *testing.T) {
	var (
		data = []byte("this is a very normal string.")
		nP   = 2
		nD   = 4
	)
	enc, err := reedsolomon.New(nD, nP)
	assert.Nil(t, err)
	shards, err := makeShards(enc, data)
	assert.Nil(t, err)
	assert.Equal(t, nP+nD, len(shards))
	reqs, err := makeProofRequests(shards)
	assert.Nil(t, err)
	assert.Equal(t, nP+nD, len(reqs))
	for _, r := range reqs {
		assert.True(t, validateProof(r))
	}
}

// This test should be run repeatedly for some time.
func TestRBCRandomized(t *testing.T) {
	if err := testRBCRandomized(t); err != nil {
		t.Fatalf("Failed, reason=%+v", err)
	}
}
func testRBCRandomized(t *testing.T) error {
	var err error
	var N, T = 7, 5

	msgs := make([]*testRBCMsg, 0)
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

	var input [10000]byte
	rand.Read(input[:])

	rbc := make([]*RBC, N)
	proposerID := uint64(rand.Intn(N))
	for i := range rbc {
		rbc[i] = NewRBC(cfg[i], proposerID)
	}
	var inMsgs []*BroadcastMessage
	if inMsgs, err = rbc[proposerID].InputValue(input[:]); err != nil {
		return fmt.Errorf("Failed to process RBC.InputValue: %v", err)
	}
	msgs = appendTestRBCMsgsExplicit(inMsgs, proposerID, nodes, msgs)
	for len(msgs) != 0 {
		m := rand.Intn(len(msgs))
		msg := msgs[m]
		msgTo := msg.to
		if err = rbc[msgTo].HandleMessage(msg.from, msg.msg); err != nil {
			return fmt.Errorf("Failed to RBC.HandleMessage: %v", err)
		}

		// Remove the message from the buffer and add the new messages.
		msgs[m] = msgs[len(msgs)-1]
		msgs = msgs[:len(msgs)-1]
		msgs = appendTestRBCMsgsBroadcast(rbc[msgTo].Messages(), msgTo, nodes, msgs)
	}

	for i := range rbc {
		out := rbc[i].Output()
		require.NotNil(t, out)
		require.Equal(t, bytes.Compare(input[:], out[:len(input)]), 0) // RBC adds zeros to the end.
		rbc[i].Stop()
	}
	return nil
}

type testRBCMsg struct {
	from uint64
	to   uint64
	msg  *BroadcastMessage
}

func appendTestRBCMsgsExplicit(msgs []*BroadcastMessage, senderID uint64, nodes []uint64, buf []*testRBCMsg) []*testRBCMsg {
	output := buf[:]
	msgPos := 0
	for n := range nodes {
		if nodes[n] != senderID {
			output = append(output, &testRBCMsg{from: senderID, to: nodes[n], msg: msgs[msgPos]})
			msgPos++
		}
	}
	return output
}
func appendTestRBCMsgsBroadcast(msgs []*BroadcastMessage, senderID uint64, nodes []uint64, buf []*testRBCMsg) []*testRBCMsg {
	output := buf[:]
	for n := range nodes {
		if nodes[n] != senderID {
			for m := range msgs {
				output = append(output, &testRBCMsg{from: senderID, to: nodes[n], msg: msgs[m]})
			}
		}
	}
	return output
}

type bcResult struct {
	nodeID uint64
	value  []byte
}

// simple engine to test RBC independently.
type testRBCEngine struct {
	faulty    bool
	rbc       *RBC
	rpcCh     <-chan RPC
	resCh     chan bcResult
	transport Transport
}

func newTestRBCEngine(resCh chan bcResult, rbc *RBC, tr Transport) *testRBCEngine {
	return &testRBCEngine{
		rbc:       rbc,
		rpcCh:     tr.Consume(),
		resCh:     resCh,
		transport: tr,
	}
}

func (e *testRBCEngine) run() {
	for {
		select {
		case rpc := <-e.rpcCh:
			err := e.rbc.HandleMessage(rpc.NodeID, rpc.Payload.(*BroadcastMessage))
			if err != nil {
				log.Println(err)
				continue
			}
			for _, msg := range e.rbc.Messages() {
				if err := e.transport.Broadcast(e.rbc.ID, msg); err != nil {
					panic(err)
				}
			}
			if output := e.rbc.Output(); output != nil {
				// Faulty node will refuse to send its produced output, causing
				// potential disturb of consensus liveness.
				if e.faulty {
					continue
				}
				e.resCh <- bcResult{
					nodeID: e.rbc.ID,
					value:  output,
				}
			}
		}
	}
}

func (e *testRBCEngine) inputValue(data []byte) error {
	reqs, err := e.rbc.InputValue(data)
	if err != nil {
		return err
	}
	msgs := make([]interface{}, len(reqs))
	for i := 0; i < len(reqs); i++ {
		msgs[i] = reqs[i]
	}
	return e.transport.SendProofMessages(e.rbc.ID, msgs)
}

func makeRBCNodes(n, pid int, resCh chan bcResult) []*testRBCEngine {
	transports := makeTransports(n)
	connectTransports(transports)
	nodes := make([]*testRBCEngine, len(transports))

	for i, tr := range transports {
		cfg := Config{
			ID: uint64(i),
			N:  len(transports),
			F:  -1,
		}
		nodes[i] = newTestRBCEngine(resCh, NewRBC(cfg, uint64(pid)), tr)
		go nodes[i].run()
	}
	return nodes
}

func connectTransports(tt []Transport) {
	for i := 0; i < len(tt); i++ {
		for ii := 0; ii < len(tt); ii++ {
			if ii == i {
				continue
			}
			tt[i].Connect(tt[ii].Addr(), tt[ii])
		}
	}
}
