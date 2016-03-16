package shared

import (
	"regexp"
	"continuedb/globalpb"
	"time"
	log "github.com/Sirupsen/logrus"
	"errors"
)

var errNotFresh error = errors.New("The info is not fresh.")

type Callback func(key string, val globalpb.Value)

type callback struct {
	pattern *regexp.Regexp
	f Callback;
}

// Note: not thread safe.
type store struct {
	meta *globalpb.NodeMeta
	infos map[string]*Info
	nodes map[uint64]*Node
	callbacks []*callback
}

func newStore(meta *globalpb.NodeMeta) *store {
	return &store{
		meta: meta,
		infos: make(map[string]*Info),
		nodes: make(map[uint64]*Node),
	}
}

func (s *store) visitInfos(f func(string, *Info) error) error {
	now := time.Now().UnixNano()

	for k, i := range s.infos {
		if i.isExpired(now) {
			delete(s.infos, k)
			continue
		}
		if err := f(k, i); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) getNodes() map[uint64]*Node {
	copy := make(map[uint64]*Node)
	for id, meta := range s.nodes {
		copy[id] = meta
	}
	return copy
}

func (s *store) delta(nodes map[uint64]*Node) map[string]*Info {
	ret:= make(map[string]*Info)
	err := s.visitInfos(func(key string, info *Info) error {
		if info.isFresh(s.meta.Id, nodes[info.NodeId]) {
			ret[key] = info
		}
		return nil
	})
	if err != nil {
		// This should never happen.
		panic(err)
	}

	return ret
}

// combine combines delta with itself infos,
// return the number of fresh infos.
func (s *store) combine(delta map[string]*Info) (int, error) {
	var (
		freshCount int
		err error
	)
	for k, i := range delta {
		copy := *i
		copy.Hops++
		copy.PeerId = s.meta.Id
		if copy.OrigStamp == 0 {
			// This should never happen.
			panic("Combine a info which node id is 0.")
		}
		if addErr := s.addInfo(k, &copy); addErr == nil {
			freshCount++
		} else if addErr != errNotFresh {
			err = addErr
		}
	}
	return freshCount, err
}

func (s *store) addInfo(key string, info *Info) error {
	if info.NodeId == 0 {
		// This should never happen.
		panic("Info with 0 node id.")
	}
	if i, ok := s.infos[key]; ok {
		existingTime := i.Value.Timestamp.WallTime
		newTime := i.Value.Timestamp.WallTime
		if existingTime > newTime || (existingTime == newTime && i.Hops <= info.Hops) {
			return errNotFresh
		}
	}
	s.infos[key] = info
	if node, ok := s.nodes[info.NodeId]; ok {
		if node.HighWaterStamp < info.OrigStamp {
			node.HighWaterStamp = info.OrigStamp
		}
		if node.MinHops > info.Hops {
			node.MinHops = info.Hops
		}
	} else {
		s.nodes[info.NodeId] = &Node{info.OrigStamp, info.Hops}
	}

	return nil
}

func (s *store) getInfo(key string) *Info {
	if i, ok := s.infos[key]; ok {
		if i.isExpired(time.Now().UnixNano()) {
			delete(s.infos, key)
		} else {
			return i
		}
	}
	return nil
}

func (s *store) mostDistant() (uint64, int) {
	var (
		nodeId uint64
		maxHops uint32
	)
	for id, n := range s.nodes {
		if n.MinHops > maxHops {
			maxHops = n.MinHops
			nodeId = id
		}
	}
	return nodeId, int(maxHops)
}

func (s *store) leastUseful(nodes *nodeSet) uint64 {
	statistics := make(map[uint64]int)
	for id, _ := range nodes.nodes {
		statistics[id] = 0
	}
	err := s.visitInfos(func(_ string, info *Info) error {
		if _, ok := nodes.nodes[info.NodeId]; ok {
			statistics[info.NodeId]++
		}
		return nil
	})
	if err != nil {
		// This should never happen
		panic(err)
	}
	var (
		nodeId uint64
		minCnt int
	)
	for id, cnt := range statistics {
		if cnt < minCnt {
			minCnt = cnt
			nodeId = id
		}
	}
	return nodeId
}

// registerCallback registers a callback function for the specified pattern
// and return a function to unregister it.
func (s *store) registerCallback(pattern string, f Callback) func() {
	cb := &callback{regexp.MustCompile(pattern), f}
	s.callbacks = append(s.callbacks, cb)

	err := s.visitInfos(func(key string, info *Info) error {
		if cb.pattern.MatchString(key) {
			// Run in a goroutine to avoid mutex reentry.
			go cb.f(key, *info.Value)
		}
		return nil
	})
	if err != nil {
		// This should never happen.
		panic(err)
	}

	log.Infof("Callback(%v) registered.", cb)

	return func() {
		for i := range s.callbacks {
			if s.callbacks[i] == cb {
				n := len(s.callbacks)
				s.callbacks[i], s.callbacks[n-1] = s.callbacks[n-1], s.callbacks[i]
				s.callbacks = s.callbacks[:n-1]
				log.Infof("Callback(%v) unregistered.", cb)
				break
			}
		}
	}
}

func (s *store) processCallback(key string, value globalpb.Value) {
	var matches []Callback
	for _, cb := range s.callbacks {
		if cb.pattern.MatchString(key) {
			matches = append(matches, cb.f)
		}
	}
	go func() {
		for _, f := range matches {
			f(key, value)
		}
	}()
}
