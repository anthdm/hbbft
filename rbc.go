package hbbft

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/NebulousLabs/merkletree"
	"github.com/klauspost/reedsolomon"
)

// Config holds hyperparameters used in each (sub)protocol.
type Config struct {
	// Unique id of the node
	ID uint64
	// Number of nodes and number of tolerate faulty nodes.
	N, F int
	// The underlying transport for exchanging messages.
	Transport Transport
}

// BroadcastMessage holds the payload sent between nodes in the rbc protocol.
// Its basically just a wrapper to let top-level protocols distinguish incoming
// messages.
type BroadcastMessage struct {
	Payload interface{}
}

// BroadcastOutput holds the output after processing an BroadcastMessage.
type BroadcastOutput struct {
	// Value is the possible outcome of the rbc protocol.
	Value []byte
	// Messages holds a slice of BroadcastMessages that need to be broadcasted after
	// processing the message. This makes testing the messages transport independent.
	// And allows returned messages to be propagated to a top level protocol.
	Messages []*BroadcastMessage
}

// ProofRequest holds the RootHash along with the Shard of the erasure encoded
// payload.
type ProofRequest struct {
	RootHash []byte
	// Proof[0] will containt the actual data.
	Proof         [][]byte
	Index, Leaves int
}

// EchoRequest represents the echoed version of the proof.
type EchoRequest struct {
	ProofRequest
}

// ReadyRequest holds the RootHash of the received proof and should be sent
// after receiving and validating enough proof chunks.
type ReadyRequest struct {
	RootHash []byte
}

type proofs []ProofRequest

func (p proofs) Len() int           { return len(p) }
func (p proofs) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p proofs) Less(i, j int) bool { return p[i].Index < p[j].Index }

// RBC represents the instance of the "Reliable Broadcast Algorithm".
type RBC struct {
	// Config holds the configuration.
	Config

	// proposerID is the ID of the proposing node of this RB instance.
	proposerID uint64

	// The reedsolomon encoder to encode the proposed value into shards.
	enc reedsolomon.Encoder

	// recvReadys is a mapping between the sender and the root hash that was
	// inluded in the ReadyRequest.
	recvReadys map[uint64][]byte

	// revcEchos is a mapping between the sender and the EchoRequest.
	recvEchos map[uint64]*EchoRequest

	// Number of the parity and data shards that will be used for erasure encoding
	// the given value.
	numParityShards, numDataShards int

	echoSent, readySent, outputDecoded bool
}

// NewRBC returns a new instance of the ReliableBroadcast configured
// with the given config
func NewRBC(cfg Config) *RBC {
	if cfg.F == 0 {
		cfg.F = (cfg.N - 1) / 3
	}
	var (
		parityShards = 2 * cfg.F
		dataShards   = cfg.N - parityShards
	)
	enc, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		panic(err)
	}
	return &RBC{
		Config:          cfg,
		recvEchos:       make(map[uint64]*EchoRequest),
		recvReadys:      make(map[uint64][]byte),
		enc:             enc,
		numParityShards: parityShards,
		numDataShards:   dataShards,
	}
}

// Propose will propose the given data as value V. The data will first splitted
// into shards and additional parity shards (used for reconstruction), the
// equally splitted shards will be fed into a reedsolomon encoder. After encoding,
// equal requests are made to each participant in the network.
func (r *RBC) Propose(data []byte) error {
	shards, err := makeShards(r.enc, data)
	if err != nil {
		return err
	}
	reqs, err := makeProofRequests(shards)
	if err != nil {
		return err
	}

	// The first request is for ourselfs. The rests is distributed under the
	// participants.
	r.handleProofRequest(r.ID, reqs[0])
	go r.Transport.SendProofRequests(r.ID, reqs[1:])
	return nil
}

// HandleMessage will process the given rpc message. The caller is resposible to
// make sure only RPC messages are passed that are elligible for the RBC protocol.
func (r *RBC) HandleMessage(senderID uint64, msg BroadcastMessage) (*BroadcastOutput, error) {
	switch t := msg.Payload.(type) {
	case *ProofRequest:
		return r.handleProofRequest(senderID, t)
	case *EchoRequest:
		return r.handleEchoRequest(senderID, t)
	case *ReadyRequest:
		return r.handleReadyRequest(senderID, t)
	default:
		return nil, fmt.Errorf("invalid RBC protocol message: %v", msg)
	}
}

// When a node receives a Proof from a proposer it broadcasts the proof as an
// EchoRequest to the network after validating its content.
func (r *RBC) handleProofRequest(senderID uint64, req *ProofRequest) ([]byte, error) {
	if senderID != r.proposerID {
		return nil, fmt.Errorf(
			"receiving proof from (%d) that is not from the proposing node (%d)",
			senderID, r.proposerID,
		)
	}
	if r.echoSent {
		return nil, fmt.Errorf("received proof from (%d) more the once", senderID)
	}
	if !validateProof(req) {
		return nil, fmt.Errorf("received invalid proof from (%d)", senderID)
	}
	r.echoSent = true
	echo := &EchoRequest{*req}
	go r.Transport.Broadcast(r.ID, echo)
	return r.handleEchoRequest(r.ID, echo)
}

// Every node that has received (N - f) echo's with the same root hash from
// distinct nodes knows that at least (f + 1) "good" nodes have sent an echo
// with that root hash to every participant. Upon receiving (N - f) echo's we
// broadcast a ReadyRequest with the roothash. Even without enough echos, if a
// node receives (f + 1) ReadyRequests we know that at least one good node has
// sent Ready, hence also knows that everyone will be able to decode eventually
// and broadcast ready itself.
func (r *RBC) handleEchoRequest(senderID uint64, req *EchoRequest) ([]byte, error) {
	if _, ok := r.recvEchos[senderID]; ok {
		return nil, fmt.Errorf("received multiple echos from (%d)", senderID)
	}
	if !validateProof(&req.ProofRequest) {
		return nil, fmt.Errorf("received invalid proof from (%d)", senderID)
	}

	r.recvEchos[senderID] = req
	if r.readySent || r.countEchos(req.RootHash) < r.N-r.F {
		return r.maybeDecodeValue(req.RootHash), nil
	}

	r.readySent = true
	ready := &ReadyRequest{req.RootHash}
	go r.Transport.Broadcast(r.ID, ready)
	return r.handleReadyRequest(r.ID, ready)
}

// If a node had received (2 * f + 1) ready's (with matching root hash)
// from distinct nodes, it knows that at least (f + 1) good nodes have sent
// it. Hence every good node will eventually receive (f + 1) and broadcast
// ready itself. Eventually a node with (2 * f + 1) readys and (f + 1) echos
// will decode and ouput the value, knowing that every other good node will
// do the same.
func (r *RBC) handleReadyRequest(senderID uint64, req *ReadyRequest) ([]byte, error) {
	if _, ok := r.recvReadys[senderID]; ok {
		return nil, fmt.Errorf("received multiple readys from (%d)", senderID)
	}
	r.recvReadys[senderID] = req.RootHash

	if r.countReadys(req.RootHash) == r.F+1 && !r.readySent {
		r.readySent = true
		go r.Transport.Broadcast(r.ID, &ReadyRequest{req.RootHash})
	}
	return r.maybeDecodeValue(req.RootHash), nil
}

// maybeDecodeValue will check whether the Value (V) can be decoded from the received
// shards. Returns nil if not, value (V) if decoded has succeed.
func (r *RBC) maybeDecodeValue(hash []byte) []byte {
	if r.outputDecoded || r.countReadys(hash) <= 2*r.F || r.countEchos(hash) <= r.F {
		return nil
	}

	// At this point we can decode the shards. First we create a new slice of
	// only sortable proof values.
	r.outputDecoded = true
	var prfs proofs
	for _, echo := range r.recvEchos {
		prfs = append(prfs, echo.ProofRequest)
	}
	sort.Sort(prfs)

	// Reconstruct the value with reedsolomon encoding.
	shards := make([][]byte, r.numParityShards+r.numDataShards)
	for _, p := range prfs {
		shards[p.Index] = p.Proof[0]
	}
	if err := r.enc.Reconstruct(shards); err != nil {
		return nil
	}
	var value []byte
	for _, data := range shards[:r.numDataShards] {
		value = append(value, data...)
	}
	return value
}

// countEchos count the number of echos with the given hash.
func (r *RBC) countEchos(hash []byte) int {
	n := 0
	for _, e := range r.recvEchos {
		if bytes.Compare(hash, e.RootHash) == 0 {
			n++
		}
	}
	return n
}

// countReadys count the number of readys with the given hash.
func (r *RBC) countReadys(hash []byte) int {
	n := 0
	for _, h := range r.recvReadys {
		if bytes.Compare(hash, h) == 0 {
			n++
		}
	}
	return n
}

// makeProofRequests will build a merkletree out of the given shards and make
// equal ProofRequest to send one proof to each participant in the consensus.
func makeProofRequests(shards [][]byte) ([]*ProofRequest, error) {
	reqs := make([]*ProofRequest, len(shards))
	for i := 0; i < len(reqs); i++ {
		tree := merkletree.New(sha256.New())
		tree.SetIndex(uint64(i))
		for i := 0; i < len(shards); i++ {
			tree.Push(shards[i])
		}
		root, proof, proofIndex, n := tree.Prove()
		reqs[i] = &ProofRequest{
			RootHash: root,
			Proof:    proof,
			Index:    int(proofIndex),
			Leaves:   int(n),
		}
	}
	return reqs, nil
}

// validateProof will validate the given ProofRequest and hence return true or
// false accordingly.
func validateProof(req *ProofRequest) bool {
	return merkletree.VerifyProof(
		sha256.New(),
		req.RootHash,
		req.Proof,
		uint64(req.Index),
		uint64(req.Leaves))
}

// makeShards will split the given value into equal sized shards along with
// somen additional parity shards.
func makeShards(enc reedsolomon.Encoder, data []byte) ([][]byte, error) {
	shards, err := enc.Split(data)
	if err != nil {
		return nil, err
	}
	if err := enc.Encode(shards); err != nil {
		return nil, err
	}
	return shards, nil
}
