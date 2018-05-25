package main

import (
	"encoding/binary"
	"encoding/gob"
	"log"
	"math/rand"
	"time"

	"github.com/anthdm/hbbft"
)

func main() {
	var (
		txSize = 128  // bytes/tx
		nTx    = 1000 // number of transactions in the buffers
		nNodes = 4    // number of nodes in the network
	)
	nodes := makeNodes(nNodes, nTx, txSize)
	for _, node := range nodes {
		go node.start()
	}
	time.Sleep(5 * time.Second)
	for _, node := range nodes {
		total := 0
		for _, txx := range node.hb.Outputs() {
			total += len(txx)
		}
		log.Printf("node (%d) processed a total of (%d) transactions in 5 seconds [ %d tx/s ]", node.hb.ID, total, total/5)
	}
}

type node struct {
	hb        *hbbft.HoneyBadger
	rpcCh     <-chan hbbft.RPC
	transport hbbft.Transport
}

func newNode(hb *hbbft.HoneyBadger, tr hbbft.Transport) *node {
	return &node{
		hb:        hb,
		transport: tr,
		rpcCh:     tr.Consume(),
	}
}

func (n *node) start() error {
	if err := n.hb.Start(); err != nil {
		return err
	}
	for _, msg := range n.hb.Messages() {
		n.transport.SendMessage(n.hb.ID, msg.To, msg.Payload)
	}
	return nil
}

func (n *node) run() {
	for {
		select {
		case rpc := <-n.rpcCh:
			msg := rpc.Payload.(hbbft.HBMessage)
			acsMsg := msg.Payload.(*hbbft.ACSMessage)
			if err := n.hb.HandleMessage(rpc.NodeID, msg.Epoch, acsMsg); err != nil {
				log.Printf("error occured when processing message: %s", err)
				continue
			}
			for _, msg := range n.hb.Messages() {
				n.transport.SendMessage(n.hb.ID, msg.To, msg.Payload)
				time.Sleep(1 * time.Millisecond)
			}
		}
	}
}

func makeNodes(n, ntx, txsize int) []*node {
	var (
		transports = makeTransports(n)
		nodes      = make([]*node, len(transports))
	)
	connectTransports(transports)

	for i, tr := range transports {
		cfg := hbbft.Config{
			ID:    uint64(i),
			N:     len(transports),
			Nodes: makeids(n),
		}
		nodes[i] = newNode(hbbft.NewHoneyBadger(cfg), tr)
		for ii := 0; ii < ntx; ii++ {
			nodes[i].hb.AddTransaction(newTx(txsize))
		}
		go nodes[i].run()
	}
	return nodes
}

func makeTransports(n int) []hbbft.Transport {
	transports := make([]hbbft.Transport, n)
	for i := 0; i < n; i++ {
		transports[i] = hbbft.NewLocalTransport(uint64(i))
	}
	return transports
}

func connectTransports(tt []hbbft.Transport) {
	for i := 0; i < len(tt); i++ {
		for ii := 0; ii < len(tt); ii++ {
			if ii == i {
				continue
			}
			tt[i].Connect(tt[ii].Addr(), tt[ii])
		}
	}
}

func makeids(n int) []uint64 {
	ids := make([]uint64, n)
	for i := 0; i < n; i++ {
		ids[i] = uint64(i)
	}
	return ids
}

type tx struct {
	Nonce uint64
	Data  []byte
}

// Size can be used to simulate large transactions in the network.
func newTx(size int) *tx {
	return &tx{
		Nonce: rand.Uint64(),
		Data:  make([]byte, size),
	}
}

// Hash implements the hbbft.Transaction interface.
func (t *tx) Hash() []byte {
	buf := make([]byte, 8) // sizeOf(uint64) + len(data)
	binary.LittleEndian.PutUint64(buf, t.Nonce)
	return buf
}

func init() {
	rand.Seed(time.Now().UnixNano())
	gob.Register(&tx{})
}
