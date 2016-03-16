package shared

// NOTE: not thread safe
type nodeSet struct {
	nodes map[uint64]struct{}
	maxsize int
}

func newNodeSet(maxsize int) *nodeSet {
	return &nodeSet{
		nodes: make(map[uint64]struct{}),
		maxsize: maxsize,
	}
}

func (ns *nodeSet) len() int {
	return len(ns.nodes)
}

func (ns *nodeSet) setMaxSize(size int) {
	ns.maxsize = size
}

func (ns *nodeSet) hasSpace() bool {
	return ns.len() < ns.maxsize
}

func (ns *nodeSet) addNode(nodeId uint64) {
	ns.nodes[nodeId] = struct{}{}
}

func (ns *nodeSet) removeNode(nodeId uint64) {
	if _, ok := ns.nodes[nodeId]; ok {
		delete(ns.nodes, nodeId)
	}
}

func (ns *nodeSet) hasNode(nodeId uint64) bool {
	_, ok := ns.nodes[nodeId]
	return ok
}

func (ns *nodeSet) asSlice() []uint64 {
	ret := make([]uint64, 0, len(ns.nodes))
	for k := range ns.nodes {
		ret = append(ret, k)
	}
	return ret
}
