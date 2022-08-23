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
	benchmark(4, 128, 100)
	benchmark(6, 128, 200)
	benchmark(8, 128, 400)
	benchmark(12, 128, 1000)
}

type message struct {
	from    uint64
	payload hbbft.MessageTuple
}

func benchmark(n, txsize, batchSize int) {
	log.Printf("Starting benchmark %d nodes %d tx size %d batch size over 5 seconds...", n, txsize, batchSize)
	var (
		nodes    = makeNodes(n, 10000, txsize, batchSize)
		messages = make(chan message, 1024*1024)
	)
	for _, node := range nodes {
		if err := node.Start(); err != nil {
			log.Fatal(err)
		}
		for _, msg := range node.Messages() {
			messages <- message{node.ID, msg}
		}
	}

	timer := time.After(5 * time.Second)
running:
	for {
		select {
		case messag := <-messages:
			node := nodes[messag.payload.To]
			hbmsg := messag.payload.Payload.(hbbft.HBMessage)
			if err := node.HandleMessage(messag.from, hbmsg.Epoch, hbmsg.Payload.(*hbbft.ACSMessage)); err != nil {
				log.Fatal(err)
			}
			for _, msg := range node.Messages() {
				messages <- message{node.ID, msg}
			}
		case <-timer:
			for _, node := range nodes {
				total := 0
				for _, txx := range node.Outputs() {
					total += len(txx)
				}
				log.Printf("node (%d) processed a total of (%d) transactions in 5 seconds [ %d tx/s ]",
					node.ID, total, total/5)
			}
			break running
		default:
		}
	}
}

func makeNodes(n, ntx, txsize, batchSize int) []*hbbft.HoneyBadger {
	nodes := make([]*hbbft.HoneyBadger, n)
	for i := 0; i < n; i++ {
		cfg := hbbft.Config{
			N:         n,
			F:         -1,
			ID:        uint64(i),
			Nodes:     makeids(n),
			BatchSize: batchSize,
		}
		nodes[i] = hbbft.NewHoneyBadger(cfg)
		for ii := 0; ii < ntx; ii++ {
			nodes[i].AddTransaction(newTx(txsize))
		}
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
