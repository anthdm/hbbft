package hbbft

import (
	"log"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test ACS with 4 good nodes. The result should be that at least the output
// of (N - f) nodes has been provided.
func TestACSWithNormalNodes(t *testing.T) {
	var (
		resultCh = make(chan map[uint64][]byte)
		nodes    = makeACSNodes(4, 0, resultCh)
		wg       sync.WaitGroup
	)

	inputs := map[int][]byte{
		0: []byte("AAAAAA"),
		1: []byte("BBBBBB"),
		2: []byte("CCCCCC"),
		3: []byte("DDDDDD"),
	}

	wg.Add(len(nodes))
	go func() {
		for {
			res := <-resultCh
			assert.Equal(t, len(nodes), len(res))
			for id, result := range res {
				assert.Equal(t, inputs[int(id)], result)
			}
			wg.Done()
		}
	}()

	for nodeID, value := range inputs {
		assert.Nil(t, nodes[nodeID].inputValue(value))
	}
	wg.Wait()
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
			for _, msg := range n.acs.messageQue.messages() {
				go n.transport.SendMessage(n.acs.ID, msg.to, msg.payload)
			}
		}
	}
}

func (n *testACSNode) inputValue(value []byte) error {
	if err := n.acs.InputValue(value); err != nil {
		return err
	}
	for _, msg := range n.acs.messageQue.messages() {
		go n.transport.SendMessage(n.acs.ID, msg.to, msg.payload)
	}
	// time.Sleep(10 * time.Millisecond)
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
			N:     len(nodes),
			ID:    uint64(i),
			Nodes: ids,
		}
		nodes[i] = newTestACSNode(NewACS(cfg), transports[i], resultCh)
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
		transports[i] = NewLocalTransport(uint64(i))
	}
	return transports
}
