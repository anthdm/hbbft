package hbbft

import (
	"log"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalBBARound(t *testing.T) {
	transports := makeTransports(4)
	connectTransports(transports)

	var (
		ee    = make([]testBBAEngine, len(transports))
		resCh = make(chan bbaResult)
	)

	for i, tr := range transports {
		ee[i] = newTestBBAEngine(resCh,
			NewBBA(
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
			assert.Equal(t, true, res.value)
			wg.Done()
		}
	}()

	// Let the first node propose a value to the others.
	for _, bba := range ee {
		err := bba.propose(true)
		assert.Nil(t, err)
	}
	wg.Wait()
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

type bbaResult struct {
	nodeID uint64
	value  bool
}

type testBBAEngine struct {
	bba       *BBA
	rpcCh     <-chan RPC
	resCh     chan bbaResult
	transport Transport
}

func newTestBBAEngine(resCh chan bbaResult, bba *BBA, tr Transport) testBBAEngine {
	return testBBAEngine{
		bba:       bba,
		rpcCh:     tr.Consume(),
		resCh:     resCh,
		transport: tr,
	}
}

func (e testBBAEngine) run() {
	for {
		select {
		case rpc := <-e.rpcCh:
			val, err := e.bba.HandleMessage(rpc.NodeID, *rpc.Payload.(*AgreementMessage))
			if err != nil {
				log.Println(err)
				continue
			}
			if val == nil {
				continue
			}
			for _, msg := range val.Messages {
				if msg != nil {
					go e.transport.Broadcast(e.bba.ID, msg)
				}
			}
			if val.Decision != nil {
				e.resCh <- bbaResult{e.bba.ID, val.Decision.(bool)}
			}
		}
	}
}

func (e testBBAEngine) propose(value bool) error {
	res, err := e.bba.InputValue(value)
	if err != nil {
		return err
	}
	go e.transport.Broadcast(e.bba.ID, res)
	return nil
}
