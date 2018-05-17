package hbbft

import (
	"fmt"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestACSWithNormalNodes(t *testing.T) {
	var (
		resultCh = make(chan map[uint64][]byte)
		nodes    = makeACSNodes(4, 0, resultCh)
		done     chan struct{}
	)
	go func() {
		for {
			res := <-resultCh
			_ = res
		}
	}()
	assert.Nil(t, nodes[0].inputValue([]byte("aaaaaaaaaa")))
	assert.Nil(t, nodes[1].inputValue([]byte("bbbbbbbbbb")))
	assert.Nil(t, nodes[2].inputValue([]byte("cccccccccc")))
	assert.Nil(t, nodes[3].inputValue([]byte("dddddddddd")))
	<-done
}

func TestNewACS(t *testing.T) {
	nodes := []uint64{1, 2, 3}
	acs := NewACS(Config{
		N:  4,
		ID: 4,
	}, nodes)
	assert.Equal(t, 4, len(acs.bbaInstances))
	assert.Equal(t, 4, len(acs.rbcInstances))

	for i := range acs.rbcInstances {
		_, ok := acs.bbaInstances[i]
		assert.True(t, ok)
	}
	for i := range acs.bbaInstances {
		_, ok := acs.bbaInstances[i]
		assert.True(t, ok)
	}
}

type testACSNode struct {
	acs       *ACS
	transport Transport
	resultCh  chan map[uint64][]byte
	rpcCh     <-chan RPC
}

func newTestACSNode(acs *ACS, tr Transport, resultCh chan map[uint64][]byte) *testACSNode {
	return &testACSNode{
		resultCh:  resultCh,
		acs:       acs,
		transport: tr,
		rpcCh:     tr.Consume(),
	}
}

func (n *testACSNode) run() {
	for {
		select {
		case rpc := <-n.rpcCh:
			msg := rpc.Payload.(*ACSMessage)
			if err := n.acs.HandleMessage(rpc.NodeID, msg); err != nil {
				log.Println(err)
				continue
			}
			if output := n.acs.Output(); output != nil {
				n.resultCh <- output
				log.Printf("ACS (%d) outputed his result %v", n.acs.ID, output)
			}
			for _, msg := range n.acs.Messages() {
				go n.transport.Broadcast(n.acs.ID, msg)
			}
		}
	}
}

func (n *testACSNode) inputValue(value []byte) error {
	reqs, msgs, err := n.acs.InputValue(value)
	if err != nil {
		return err
	}
	mm := make([]interface{}, len(reqs))
	for i := 0; i < len(reqs); i++ {
		mm[i] = &ACSMessage{n.acs.ID, reqs[i]}
	}
	go n.transport.SendProofMessages(n.acs.ID, mm)
	for _, msg := range msgs {
		go n.transport.Broadcast(n.acs.ID, msg)
	}
	return nil
}

func makeACSNodes(n, pid int, resultCh chan map[uint64][]byte) []*testACSNode {
	var (
		transports = makeTransports(n)
		nodes      = make([]*testACSNode, len(transports))
		ids        = makeids(n)
	)
	connectTransports(transports)
	for i := 0; i < len(nodes); i++ {
		cfg := Config{
			N:  len(nodes),
			ID: uint64(i),
		}
		nodes[i] = newTestACSNode(NewACS(cfg, ids), transports[i], resultCh)
		go nodes[i].run()
	}
	return nodes
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
		transports[i] = NewLocalTransport(fmt.Sprintf("tr_%d", i))
	}
	return transports
}
