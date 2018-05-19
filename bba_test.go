package hbbft

import (
	"log"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// Test a round of BBA where 1 of the nodes has not completed the broadcast
// yet and hence get false as input.
func TestAgreementWithLateNode(t *testing.T) {
	var (
		pid      = 0
		nNodes   = 4
		resultCh = make(chan bool)
		nodes    = makeBBANodes(nNodes, pid, resultCh)
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

func TestBBAStepByStep(t *testing.T) {
	bba := NewBBA(Config{N: 4, ID: 0}, uint64(0))

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

// Test a round of BBA where all nodes have completed the broadcast and hence
// input all true.
func TestAgreementAllGoodNodes(t *testing.T) {
	var (
		pid      = 0
		nNodes   = 4
		resultCh = make(chan bool)
		nodes    = makeBBANodes(nNodes, pid, resultCh)
		wg       sync.WaitGroup
	)
	wg.Add(nNodes)
	go func() {
		// Expect all nodes to output true.
		for {
			res := <-resultCh
			assert.True(t, res)
			wg.Done()
		}
	}()
	// Let all nodes input true.
	assert.Nil(t, nodes[0].inputValue(true))
	assert.Nil(t, nodes[1].inputValue(true))
	assert.Nil(t, nodes[2].inputValue(true))
	assert.Nil(t, nodes[3].inputValue(true))
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

func makeBBANodes(n, pid int, resultCh chan bool) []*testBBANode {
	transports := makeTransports(n)
	connectTransports(transports)
	nodes := make([]*testBBANode, len(transports))

	for i, tr := range transports {
		cfg := Config{
			N:  len(nodes),
			ID: uint64(i),
		}
		nodes[i] = newTestBBANode(uint64(i), tr, resultCh)
		nodes[i].bba = NewBBA(cfg, uint64(pid))
		go nodes[i].run()
	}
	return nodes
}

type testBBANode struct {
	id        uint64
	bba       *BBA
	transport Transport
	rpcCh     <-chan RPC
	resultCh  chan bool
}

func newTestBBANode(id uint64, tr Transport, resultCh chan bool) *testBBANode {
	return &testBBANode{
		id:        id,
		transport: tr,
		rpcCh:     tr.Consume(),
		resultCh:  resultCh,
	}
}

func (n *testBBANode) inputValue(b bool) error {
	if err := n.bba.InputValue(b); err != nil {
		return err
	}

	for _, msg := range n.bba.Messages() {
		go n.transport.Broadcast(n.id, msg)
	}
	time.Sleep(10 * time.Millisecond)
	return nil
}

func (n *testBBANode) run() {
	for {
		select {
		case rpc := <-n.rpcCh:
			switch t := rpc.Payload.(type) {
			case *AgreementMessage:
				if err := n.bba.HandleMessage(rpc.NodeID, t); err != nil {
					log.Println(err)
				}
				for _, msg := range n.bba.Messages() {
					go n.transport.Broadcast(n.id, msg)
				}
				if out := n.bba.Output(); out != nil {
					n.resultCh <- out.(bool)
				}
			}
		}
	}
}

func init() { logrus.SetLevel(logrus.DebugLevel) }
