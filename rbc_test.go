package hbbft

import (
	"log"
	"sync"
	"testing"

	"github.com/klauspost/reedsolomon"

	"github.com/stretchr/testify/assert"
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
	rbc := NewRBC(Config{N: 4}, 0)
	output := []byte("a")
	rbc.output = output
	assert.Equal(t, output, rbc.Output())
	assert.Nil(t, rbc.Output())
}

func TestRBCMessagesIsEmptyAfterConsuming(t *testing.T) {
	rbc := NewRBC(Config{N: 4}, 0)
	rbc.messages = []*BroadcastMessage{&BroadcastMessage{}}
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
				e.transport.Broadcast(e.rbc.ID, msg)
			}
			if output := e.rbc.Output(); output != nil {
				// Faulty node will refuse to send its produced output, causing
				// potential disturb of conensus liveness.
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
	e.transport.SendProofMessages(e.rbc.ID, msgs)
	return nil
}

func makeRBCNodes(n, pid int, resCh chan bcResult) []*testRBCEngine {
	transports := makeTransports(n)
	connectTransports(transports)
	nodes := make([]*testRBCEngine, len(transports))

	for i, tr := range transports {
		cfg := Config{
			ID: uint64(i),
			N:  len(transports),
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
