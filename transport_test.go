package hbbft

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSendProofMessages(t *testing.T) {
	var (
		reqs       = make([]*BroadcastMessage, 3)
		transports = make([]Transport, 3)
		chans      = make([]<-chan RPC, 3)
	)
	for i := 0; i < len(reqs); i++ {
		reqs[i] = &BroadcastMessage{&ProofRequest{Index: i}}
		transports[i] = NewLocalTransport(uint64(i))
		chans[i] = transports[i].Consume()
	}
	// create a new transport that will do the request.
	sendingTrans := NewLocalTransport(9849)
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
				assert.IsType(t, &BroadcastMessage{}, rpc.Payload)
				assert.IsType(
					t,
					&ProofRequest{},
					rpc.Payload.(*BroadcastMessage).Payload,
				)
				wg.Done()
			}
		}(i)
	}

	// Let the sending transport send out the request.
	messages := make([]interface{}, len(reqs))
	for i, msg := range reqs {
		messages[i] = msg
	}
	sendingTrans.SendProofMessages(1337, messages)
	wg.Wait()
}

func TestBroadcast(t *testing.T) {
	var (
		transports = make([]Transport, 3)
		chans      = make([]<-chan RPC, 3)
		value      = []byte("foobar")
	)
	for i := 0; i < len(transports); i++ {
		transports[i] = NewLocalTransport(uint64(i))
		chans[i] = transports[i].Consume()
	}
	// create a new transport that will do the request.
	sendingTrans := NewLocalTransport(98499)
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
