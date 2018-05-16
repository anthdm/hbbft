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

	// SendProofMessages will equally spread the given messages under the
	// participating nodes.
	SendProofMessages(from uint64, msgs []interface{}) error

	// Broadcast multicasts the given messages to all connected nodes.
	Broadcast(from uint64, msg interface{}) error

	// Connect is used to connect this tranport to another transport.
	Connect(string, Transport)

	// Addr returns the address of the transport.
	Addr() string
}
