package main

// type node struct {
// 	hb        *hbbft.HoneyBadger
// 	rpcCh     <-chan hbbft.RPC
// 	transport hbbft.Transport
// }

// func (n *node) run() {
// 	for {
// 		select {
// 		case rpc := <-n.rpcCh:
// 			msg := rpc.Payload.(*hbbft.HBMessage)
// 			acsMsg := msg.Payload.(*hbbft.ACSMessage)
// 			if err := n.hb.HandleMessage(rpc.NodeID, msg.Epoch, acsMsg); err != nil {
// 				log.Printf("error occured when processing message: %s", err)
// 				continue
// 			}
// 			for _, msg := range n.hb.Messages() {
// 				n.transport.SendMessage(n.hb.ID, msg.To, msg.Payload)
// 			}
// 		}
// 	}
// }

// func makeNodes(n int) []*node {

// }

// func maketNodes(n int) []*node {
// 	var (
// 		transports = makeTransports(n)
// 		nodes      = make([]*testNode, len(transports))
// 	)
// 	connectTransports(transports)

// 	for i, tr := range transports {
// 		cfg := Config{
// 			ID:    uint64(i),
// 			N:     len(transports),
// 			Nodes: makeids(n),
// 		}
// 		nodes[i] = newTestNode(NewHoneyBadger(cfg), tr)
// 		nTx := 10000
// 		for ii := 0; ii < nTx; ii++ {
// 			nodes[i].hb.AddTransaction(&tx{uint64(ii)})
// 		}
// 		go nodes[i].run()
// 	}
// 	return nodes
// }

// func makeTransports(n int) []Transport {
// 	transports := make([]Transport, n)
// 	for i := 0; i < n; i++ {
// 		transports[i] = NewLocalTransport(uint64(i))
// 	}
// 	return transports
// }
