package hbbft

import (
	"log"
	"sync"
	"testing"

	"github.com/klauspost/reedsolomon"

	"github.com/stretchr/testify/assert"
)

// This tests one full round of RBC where all the nodes are behaving correctly.
func TestOneNormalBroadcastRound(t *testing.T) {
	transports := []Transport{
		NewLocalTransport("a"),
		NewLocalTransport("b"),
		NewLocalTransport("c"),
		NewLocalTransport("d"),
	}
	connectTransports(transports)

	var (
		ee    = make([]testEngine, len(transports))
		resCh = make(chan bcResult)
		value = []byte("foo bar foobar")
	)

	for i, tr := range transports {
		ee[i] = newTestEngine(resCh,
			NewRBC(
				Config{
					ID:        uint64(i),
					N:         len(transports),
					Transport: tr,
				},
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
	ee[0].propose(value)
	wg.Wait()
}

func TestNewReliableBroadcast(t *testing.T) {
	assertState := func(t *testing.T, rb *RBC, cfg Config) {
		assert.NotNil(t, rb.enc)
		assert.NotNil(t, rb.recvEchos)
		assert.NotNil(t, rb.recvReadys)
		assert.Equal(t, rb.numParityShards, cfg.F*2)
		assert.Equal(t, rb.numDataShards, cfg.N-rb.numParityShards)
	}

	cfg := Config{N: 4, F: 1}
	rb := NewRBC(cfg)
	assertState(t, rb, cfg)

	cfg = Config{N: 18, F: 4}
	rb = NewRBC(cfg)
	assertState(t, rb, cfg)

	cfg = Config{N: 100, F: 10}
	rb = NewRBC(cfg)
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

type testEngine struct {
	rbc   *RBC
	rpcCh <-chan RPC
	resCh chan bcResult
}

func newTestEngine(resCh chan bcResult, rbc *RBC, tr Transport) testEngine {
	return testEngine{
		rbc:   rbc,
		rpcCh: tr.Consume(),
		resCh: resCh,
	}
}

func (e testEngine) run() {
	for {
		select {
		case rpc := <-e.rpcCh:
			val, err := e.rbc.HandleMessage(rpc.NodeID, BroadcastMessage{rpc.Payload})
			if err != nil {
				log.Println(err)
			}
			if val != nil {
				e.resCh <- bcResult{
					nodeID: e.rbc.ID,
					value:  val,
				}
			}
		}
	}
}

func (e testEngine) propose(data []byte) {
	e.rbc.Propose(data)
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
