package hbbft

import (
	"log"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test a round of BBA where 1 of the nodes has not completed the broadcast
// yet and hence get false as input.
func TestAgreementWithLateNode(t *testing.T) {
	var (
		pid      = 0
		nNodes   = 4
		resultCh = make(chan bool, nNodes)
		nodes    = makeBBANetwork(nNodes, pid, resultCh)
		wg       sync.WaitGroup
	)
	wg.Add(nNodes)
	go func() {
		// Expect all nodes to output true.
		for res := range resultCh {
			assert.True(t, res)
			wg.Done()
		}
	}()
	assert.Nil(t, nodes[0].inputValue(true))
	assert.Nil(t, nodes[1].inputValue(true))
	assert.Nil(t, nodes[2].inputValue(true))
	assert.Nil(t, nodes[3].inputValue(false))
	wg.Wait()
}

// Test a round of BBA where all nodes have completed the broadcast and hence
// input all true.
func TestAgreementWithGoodNodes(t *testing.T) {
	var (
		pid      = 0
		nNodes   = 4
		resultCh = make(chan bool, nNodes)
		nodes    = makeBBANetwork(nNodes, pid, resultCh)
		wg       sync.WaitGroup
	)
	wg.Add(nNodes)
	go func() {
		// Expect all nodes to output true.
		for res := range resultCh {
			assert.True(t, res)
			wg.Done()
		}
	}()
	for _, node := range nodes {
		assert.Nil(t, node.inputValue(true))
	}
	wg.Wait()
}

func TestNewBBA(t *testing.T) {
	cfg := Config{N: 4}
	bba := NewBBA(cfg, 0)
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

func makeBBANetwork(n, pid int, resultCh chan bool) []*testNode {
	transports := makeTransports(n)
	connectTransports(transports)
	nodes := make([]*testNode, len(transports))
	for i := 0; i < len(nodes); i++ {
		cfg := Config{
			N:  len(nodes),
			ID: uint64(i),
		}
		nodes[i] = newTestNode(uint64(i), transports[i], resultCh)
		nodes[i].bba = NewBBA(cfg, uint64(pid))
		go nodes[i].run()
	}
	return nodes
}

type testNode struct {
	id        uint64
	bba       *BBA
	rbc       *RBC
	transport Transport
	rpcCh     <-chan RPC
	resultCh  chan bool
}

func newTestNode(id uint64, tr Transport, resultCh chan bool) *testNode {
	return &testNode{
		id:        id,
		transport: tr,
		rpcCh:     tr.Consume(),
		resultCh:  resultCh,
	}
}

func (n *testNode) inputValue(b bool) error {
	msg, err := n.bba.InputValue(b)
	if err != nil {
		return err
	}
	go n.transport.Broadcast(n.id, msg)
	return nil
}

func (n *testNode) run() {
	for {
		select {
		case rpc := <-n.rpcCh:
			switch t := rpc.Payload.(type) {
			case *AgreementMessage:
				if err := n.bba.HandleMessage(rpc.NodeID, t); err != nil {
					log.Println(err)
				}
				if out := n.bba.Output(); out != nil {
					n.resultCh <- out.(bool)
				}
				for _, msg := range n.bba.Messages() {
					go n.transport.Broadcast(n.id, msg)
				}
			}
		}
	}
}
