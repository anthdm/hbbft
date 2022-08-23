package hbbft

import (
	"encoding/gob"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEngineAddTransaction(t *testing.T) {
	cfg := Config{
		N:     6,
		F:     -1,
		ID:    0,
		Nodes: []uint64{0, 1, 2, 3, 4, 5, 6},
	}
	e := NewHoneyBadger(cfg)
	nTx := 100
	for i := 0; i < nTx; i++ {
		e.AddTransaction(&tx{uint64(i)})
	}
	assert.Equal(t, nTx, e.txBuffer.len())
}

type testNode struct {
	hb        *HoneyBadger
	rpcCh     <-chan RPC
	transport Transport
}

func newTestNode(hb *HoneyBadger, tr Transport) *testNode {
	return &testNode{
		hb:        hb,
		transport: tr,
		rpcCh:     tr.Consume(),
	}
}

func (n *testNode) run() {
	for {
		select {
		case rpc := <-n.rpcCh:
			switch t := rpc.Payload.(type) {
			case HBMessage:
				if err := n.hb.HandleMessage(rpc.NodeID, t.Epoch, t.Payload.(*ACSMessage)); err != nil {
					log.Println(err)
					continue
				}
				for _, msg := range n.hb.messageQue.messages() {
					n.transport.SendMessage(n.hb.ID, msg.To, msg.Payload)
				}
			}
		}
	}
}

func (n *testNode) propose() error {
	if err := n.hb.propose(); err != nil {
		return err
	}
	for _, msg := range n.hb.messageQue.messages() {
		n.transport.SendMessage(n.hb.ID, msg.To, msg.Payload)
	}
	return nil
}

func makeTestNodes(n int) []*testNode {
	var (
		transports = makeTransports(n)
		nodes      = make([]*testNode, len(transports))
	)
	connectTransports(transports)

	for i, tr := range transports {
		cfg := Config{
			ID:    uint64(i),
			N:     len(transports),
			F:     -1,
			Nodes: makeids(n),
		}
		nodes[i] = newTestNode(NewHoneyBadger(cfg), tr)
		nTx := 10000
		for ii := 0; ii < nTx; ii++ {
			nodes[i].hb.AddTransaction(&tx{uint64(ii)})
		}
		go nodes[i].run()
	}
	return nodes
}

func init() {
	gob.Register(&tx{})
}
