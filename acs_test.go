package hbbft

import (
	"fmt"
	"log"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TODO
// func TestNormalACSRound(t *testing.T) {
// 	transports := makeTransports(4)
// 	connectTransports(transports)

// 	var (
// 		aa    = make([]testACSEngine, len(transports))
// 		resCh = make(chan acsResult)
// 		value = []byte("the world smallest violin")
// 		nodes = []uint64{0, 1, 2, 3}
// 	)

// 	for i, tr := range transports {
// 		participants := []uint64{}
// 		for ii := 0; ii < len(nodes); ii++ {
// 			if ii == i {
// 				continue
// 			}
// 			participants = append(participants, nodes[ii])
// 		}
// 		aa[i] = newTestACSEngine(resCh,
// 			NewACS(
// 				Config{
// 					ID: uint64(i),
// 					N:  len(transports),
// 				}, participants,
// 			), tr)
// 		go aa[i].run()
// 	}

// 	var wg sync.WaitGroup
// 	wg.Add(4)
// 	// Start a routine that will collect the results from all the nodes.
// 	go func() {
// 		for {
// 			res := <-resCh
// 			t.Log(res)
// 			wg.Done()
// 		}
// 	}()

// 	aa[0].propose(value)
// 	wg.Wait()
// }

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

type acsResult struct {
	nodeID uint64
	value  []byte
}

// simple engine to test ACS independently.
type testACSEngine struct {
	acs       *ACS
	rpcCh     <-chan RPC
	resCh     chan acsResult
	transport Transport
}

func newTestACSEngine(resCh chan acsResult, acs *ACS, tr Transport) testACSEngine {
	return testACSEngine{
		acs:       acs,
		rpcCh:     tr.Consume(),
		resCh:     resCh,
		transport: tr,
	}
}

func (e testACSEngine) run() {
	for {
		select {
		case rpc := <-e.rpcCh:
			acsMessage := rpc.Payload.(*ACSMessage)
			out, err := e.acs.HandleMessage(rpc.NodeID, acsMessage)
			if err != nil {
				log.Println(err)
				continue
			}
			if out != nil {
				for _, msg := range out.Messages {
					logrus.Info(reflect.TypeOf(msg))
					go e.transport.Broadcast(e.acs.ID, &ACSMessage{acsMessage.NodeID, msg})
				}
			}
			// e.resCh <- acsResult{}
		}
	}
}

func (e testACSEngine) propose(data []byte) error {
	a, _, err := e.acs.InputValue(data)
	if err != nil {
		return err
	}
	msgs := make([]interface{}, len(a))
	for i, msg := range a {
		msgs[i] = msg
	}
	go e.transport.SendProofMessages(e.acs.ID, msgs)
	return nil
}

// makeTransports is a test helper function for making n transports.
func makeTransports(n int) []Transport {
	transports := make([]Transport, n)
	for i := 0; i < n; i++ {
		transports[i] = NewLocalTransport(fmt.Sprintf("tr_%d", i))
	}
	return transports
}
