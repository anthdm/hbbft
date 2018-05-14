package hbbft

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSendProofRequests(t *testing.T) {
	var (
		reqs       = make([]*ProofRequest, 3)
		transports = make([]Transport, 3)
		chans      = make([]<-chan RPC, 3)
	)
	for i := 0; i < len(reqs); i++ {
		reqs[i] = &ProofRequest{Index: i}
		transports[i] = NewLocalTransport(fmt.Sprintf("%d", i))
		chans[i] = transports[i].Consume()
	}
	// create a new transport that will do the request.
	sendingTrans := NewLocalTransport("A")
	transports = append(transports, sendingTrans)
	connectTransports(transports)

	// Catch and validate that everyone has received the request.
	var wg sync.WaitGroup
	for i := 0; i < len(chans); i++ {
		wg.Add(1)
		go func(i int) {
			select {
			case rpc := <-chans[i]:
				assert.Equal(t, uint64(1337), rpc.NodeID)
				assert.IsType(t, &ProofRequest{}, rpc.Payload)
				wg.Done()
			}
		}(i)
	}

	// Let the sending transport send out the request.
	sendingTrans.SendProofRequests(1337, reqs)
	wg.Wait()
}

func TestBroadcast(t *testing.T) {
	var (
		transports = make([]Transport, 3)
		chans      = make([]<-chan RPC, 3)
		value      = []byte("foobar")
	)
	for i := 0; i < len(transports); i++ {
		transports[i] = NewLocalTransport(fmt.Sprintf("%d", i))
		chans[i] = transports[i].Consume()
	}
	// create a new transport that will do the request.
	sendingTrans := NewLocalTransport("A")
	transports = append(transports, sendingTrans)
	connectTransports(transports)

	// Catch and validate that everyone has received the request.
	var wg sync.WaitGroup
	for i := 0; i < len(chans); i++ {
		wg.Add(1)
		go func(i int) {
			select {
			case rpc := <-chans[i]:
				assert.Equal(t, uint64(1337), rpc.NodeID)
				assert.IsType(t, &ReadyRequest{}, rpc.Payload)
				assert.Equal(t, value, rpc.Payload.(*ReadyRequest).RootHash)
				wg.Done()
			}
		}(i)
	}

	// Let the sending transport send out the request.
	sendingTrans.Broadcast(1337, &ReadyRequest{RootHash: value})
	wg.Wait()
}
