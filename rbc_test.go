package hbbft

import (
	"log"
	"sync"
	"testing"

	"github.com/klauspost/reedsolomon"

	"github.com/stretchr/testify/assert"
)

func TestOneNormalBroadcastRound(t *testing.T) {
	transports := makeTransports(4)
	connectTransports(transports)

	var (
		ee         = make([]testRBCEngine, len(transports))
		resCh      = make(chan bcResult)
		value      = []byte("foo bar foobar")
		proposerID uint64
	)

	for i, tr := range transports {
		ee[i] = newTestRBCEngine(resCh,
			NewRBC(
				Config{
					ID: uint64(i),
					N:  len(transports),
				}, proposerID,
			), tr)
		go ee[i].run()
	}

	var wg sync.WaitGroup
	wg.Add(4)
	// Start a routine that will collect the results from all the nodes.
	go func() {
		for {
			res := <-resCh
			assert.Equal(t, value, res.value)
			wg.Done()
		}
	}()

	// Let the first node propose a value to the others.
	err := ee[proposerID].propose(value)
	assert.Nil(t, err)
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
	rbc       *RBC
	rpcCh     <-chan RPC
	resCh     chan bcResult
	transport Transport
}

func newTestRBCEngine(resCh chan bcResult, rbc *RBC, tr Transport) testRBCEngine {
	return testRBCEngine{
		rbc:       rbc,
		rpcCh:     tr.Consume(),
		resCh:     resCh,
		transport: tr,
	}
}

func (e testRBCEngine) run() {
	for {
		select {
		case rpc := <-e.rpcCh:
			err := e.rbc.HandleMessage(rpc.NodeID, rpc.Payload.(*BroadcastMessage))
			if err != nil {
				log.Println(err)
			}
			for _, msg := range e.rbc.Messages() {
				go e.transport.Broadcast(e.rbc.ID, msg)
			}
			if output := e.rbc.Output(); output != nil {
				e.resCh <- bcResult{
					nodeID: e.rbc.ID,
					value:  output,
				}
			}
		}
	}
}

func (e testRBCEngine) propose(data []byte) error {
	reqs, err := e.rbc.InputValue(data)
	if err != nil {
		return err
	}
	msgs := make([]interface{}, len(reqs))
	for i := 0; i < len(reqs); i++ {
		msgs[i] = reqs[i]
	}
	go e.transport.SendProofMessages(e.rbc.ID, msgs)
	return nil
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
