package hbbft

// ACS implements the Asynchronous Common Subset protocol.
// ACS assumes a network of N nodes that send signed messages to each other.
// There can be f faulty nodes where (3 * f < N).
// Each participating node proposes an element for inlcusion. The protocol
// guarantees that all of the good nodes output the same set, consisting of
// at least (N -f) of the proposed values.
//
// Algorithm:
// ACS creates a Broadcast algorithm for each of the participating nodes.
// At least (N -f) of these will eventually output the element proposed by that
// node. ACS will also create and BBA instance for each participating node, to
// decide whether that node's proposed element should be inlcuded in common set.
// Whenever an element is received via broadcast, we imput "true" into the
// corresponding BBA instance. When (N-f) BBA instances have decided true we
// input false into the remaining ones, where we haven't provided input yet.
// Once all BBA instances have decided, ACS returns the set of all proposed
// values for which the decision was truthy.
type ACS struct {
	// Config holds the ACS configuration.
	Config
	// Mapping of node ids and their rbc instance.
	rbcInstances map[uint64]*RBC
	// Mapping of node ids and their bba instance.
	bbaInstances map[uint64]*BBA
	// Results of the Reliable Broadcast.
	rbcResults map[uint64][]byte
	// Results of the Binary Byzantine Agreement.
	bbaResults map[uint64]bool
}

// NewACS returns a new ACS instance configured with the given Config and node
// ids.
func NewACS(cfg Config, nodes []uint64) *ACS {
	acs := &ACS{
		rbcInstances: make(map[uint64]*RBC),
		bbaInstances: make(map[uint64]*BBA),
		rbcResults:   make(map[uint64][]byte),
		bbaResults:   make(map[uint64]bool),
	}
	// Add ourself the participating nodes.
	nodes = append(nodes, cfg.ID)
	// Create all the instances for the participating nodes
	for _, id := range nodes {
		// Configuration should be the same unless for the node id.
		cfg.ID = id
		acs.rbcInstances[id] = NewRBC(cfg)
		acs.bbaInstances[id] = NewBBA(cfg)
	}
	return acs
}

// HandleMessage handles incoming messages to ACS and redirect them to the
// appropriate sub(protocol) instance.
func (a *ACS) HandleMessage(senderID uint64, msg interface{}) {
	switch msg.(type) {
	case *AgreementMessage:
	case *BroadcastMessage:
	default:
		panic("dkjkldjflkdsjflkdjf")
	}
}

// Propose proposes the given value to each of the underlying rbc instances.
// The caller is responsible for constructing the value (V) out of its unbounded
// transaction buffer. If using this with the top level hbbft engine, the engine
// will take care of this part.
