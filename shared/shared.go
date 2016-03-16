package shared

import (
	"continuedb/globalpb"
	"continuedb/util"
	"time"
	"google.golang.org/grpc"
	"sync"
	log "github.com/Sirupsen/logrus"
	"math/rand"
	"github.com/golang/protobuf/proto"
	"errors"
)

var (
	ErrMissingKey = errors.New("The given key doesn't exist or have expired.")
)

const (
	MinPeers = 3
	MaxHops = 5
	MaxWaitTime = 30 * time.Millisecond

	ttlNodeMeta = 0 * time.Second

	bootstrapInterval = 1 * time.Second
	cullInterval = 60 * time.Second
)

// Shared is the ad-hoc node for sharing information.
// Each Shared is considered connected if it has the sentinel.
type Shared struct {
	meta *globalpb.NodeMeta
	*server
	outgoing *nodeSet

	clientsMu sync.Mutex
	clients	[]*client

	disconnect chan *client

	peers []*globalpb.NodeMeta
	peersIdx int
	triedAll bool
	bootstrapping map[uint64]struct{}
}

func NewShared(meta *globalpb.NodeMeta, peers []*globalpb.NodeMeta, grpcServer *grpc.Server) *Shared {
	return &Shared{
		meta: meta,
		server: newServer(meta, grpcServer),
		outgoing: newNodeSet(1),
		disconnect: make(chan *client, 10),
		peers: peers,
		peersIdx: len(peers) - 1,
		bootstrapping: make(map[uint64]struct{}),
	}
}

func (s *Shared) Start(worker *util.Worker) {
	s.server.start(worker)
	s.bootstrap(worker)
	s.manage(worker)

	err := s.AddInfoProto(MakeNodeIdKey(s.meta.Id), s.meta, ttlNodeMeta)
	if err != nil {
		panic(err)
	}
}

func (s *Shared) Incoming() []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.incoming.asSlice()
}

func (s *Shared) Outgoing() []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outgoing.asSlice()
}

func (s *Shared) AddInfo(key string, val []byte, ttl time.Duration) error {
	info := newInfo(s.meta.Id, val, ttl)
	s.mu.Lock()
	s.store.addInfo(key, info)
	s.mu.Unlock()
	return nil
}

func (s *Shared) GetInfo(key string) ([]byte, error) {
	s.mu.Lock()
	info := s.store.getInfo(key)
	s.mu.Unlock()
	if info != nil {
		return info.GetValue().RawBytes, nil
	}
	return nil, ErrMissingKey
}

func (s *Shared) AddInfoProto(key string, msg proto.Message, ttl time.Duration) error {
	bytes, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	return s.AddInfo(key, bytes, ttl)
}

func (s *Shared) GetInfoProto(key string, msg proto.Message) error {
	val, err := s.GetInfo(key)
	if err != nil {
		return err
	}
	return proto.Unmarshal(val, msg)
}

func (s *Shared) getNextPeersMeta() *globalpb.NodeMeta {
	if len(s.peers) == 0 {
		log.Infof("Shared(%d): No peers availiable.", s.meta.Id)
	}
	for i := 0; i < len(s.peers); i++ {
		s.peersIdx = (s.peersIdx + 1) % len(s.peers)
		if s.peersIdx == len(s.peers) - 1 {
			s.triedAll = true
		}
		meta := s.peers[s.peersIdx]
		if meta.Id == s.meta.Id {
			continue
		}
		_, active := s.bootstrapping[meta.Id]
		if !active {
			s.bootstrapping[meta.Id] = struct{}{}
			return meta
		}
	}
	return nil
}

func (s *Shared) bootstrap(worker *util.Worker) {
	worker.RunWorker(func() {
		for {
			s.mu.Lock()
			haveClients := s.outgoing.len() > 0
			haveSentinel := s.store.getInfo(KeySentinel) != nil
			if !haveClients || !haveSentinel {
				if meta := s.getNextPeersMeta(); meta != nil {
					s.startClient(meta, worker)
				}
			}
			s.mu.Unlock()

			select {
			case <-time.After(bootstrapInterval):
			case <-worker.ShouldStop():
				return
			}
		}
	})
}

func (s *Shared) manage(worker *util.Worker) {
	cullTicker := time.NewTicker(s.jitteredInterval(cullInterval))
	cullTicker.Stop()

	worker.RunWorker(func() {
		for {
			select {
			case c := <-s.disconnect:
				s.doDisconnect(c, worker)
			case <-cullTicker.C:
				s.cullNetwork()
			case <-worker.ShouldStop():
				return
			}
		}
	})
}

func (s *Shared) jitteredInterval(interval time.Duration) time.Duration {
	return time.Duration(float64(interval) * (0.75 + 0.5 * rand.Float64()))
}

func (s *Shared) cullNetwork() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.outgoing.hasSpace() {
		return
	}

	leastId := s.store.leastUseful(s.outgoing)
	if leastId != 0 {
		log.Infof("Shared(%d) stopping the least useful client(%d)", s.meta.Id, leastId)
		s.stopClient(leastId)
	}
}

func (s *Shared) doDisconnect(c *client, worker *util.Worker) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.removeClient(c)

	if c.alterMeta != nil {
		s.startClient(c.alterMeta, worker)
	}
}

func (s *Shared) startClient(meta *globalpb.NodeMeta, worker *util.Worker) {
	c := newClient(meta)
	s.clientsMu.Lock()
	s.clients = append(s.clients, c)
	s.clientsMu.Unlock()
	c.start(s, s.disconnect, worker)
}

func (s *Shared) stopClient(nodeId uint64) {
	s.clientsMu.Lock()
	s.clientsMu.Unlock()
	for i := 0; i < len(s.clients); i++ {
		if s.clients[i].serverMeta.Id == nodeId {
			s.clients[i].stop()
			break
		}
	}
}

func (s *Shared) removeClient(c *client) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for i := 0; i < len(s.clients); i++ {
		if s.clients[i] == c {
			s.clients = append(s.clients[:i], s.clients[i+1:]...)
			delete(s.bootstrapping, c.serverMeta.Id)
			s.outgoing.removeNode(c.serverMeta.Id)
			break
		}
	}
}

func (s *Shared) RegisterCallback(pattern string, callback Callback) func() {
	s.mu.Lock()
	unregister := s.store.registerCallback(pattern, callback)
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		unregister()
		s.mu.Unlock()
	}
}