package hbbft

// RPC holds the payload send between participants in the consensus.
type RPC struct {
	// NodeID is the unique identifier of the sending node.
	NodeID uint64
	// Payload beeing send.
	Payload interface{}
}

// Transport is an interface that allows the abstraction of network transports.
type Transport interface {
	// Consume returns a channel used for consuming and responding to RPC
	// requests.
	Consume() <-chan RPC

	// SendProofRequests will divide the given ProofRequests and send one to
	// each participant in the network.
	SendProofRequests(uint64, []*ProofRequest) error

	// Broadcast multicasts the given interface to each node in thenetwork.
	Broadcast(uint64, interface{}) error

	// Connect is used to connect this tranport to another transport.
	Connect(string, Transport)

	// Addr returns the address of the transport.
	Addr() string
}
