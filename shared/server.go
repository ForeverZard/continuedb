package shared

import (
	"continuedb/globalpb"
	"golang.org/x/net/context"
	log "github.com/Sirupsen/logrus"
	"errors"
	"sync"
	"continuedb/util"
	"google.golang.org/grpc"
	"strings"
"math"
	"container/list"
	"time"
	"math/rand"
)

var (
	// errUnInitialized occurs when the req's node id is 0.
	errUnInitialized = errors.New("The sender may not be initialized.")
	// errServerClosed occurs when the server has been closed.
	errServerClosed = errors.New("The server has been closed.")
)

type clientInfo struct {
	*globalpb.NodeMeta
	recentActiveTime int64
	elem *list.Element
}

type server struct {
	mu             sync.Mutex
	cond           *sync.Cond

	store          *store
	stopped        bool
	// sent & received are the count of sent and received infos.
	sent, received int
	incoming       *nodeSet

	clients        map[uint64]*clientInfo
	ll             *list.List
}

func newServer(meta *globalpb.NodeMeta, grpcServer *grpc.Server) *server {
	s := &server{
		store: newStore(meta),
		incoming: newNodeSet(1),
		clients: make(map[uint64]*clientInfo),
		ll: list.New(),
	}
	s.cond = sync.NewCond(&s.mu)

	RegisterSharedServer(grpcServer, s)

	return s
}

// cleanup removes timeout clients and wake up waiting clients.
// This should be called with the caller holding the lock.
func (s *server) cleanup() {
	for {
		e := s.ll.Back()
		if e == nil {
			break
		}
		c := e.Value.(*clientInfo)
		if c.recentActiveTime + int64(MaxWaitTime) < time.Now().UnixNano() {
			s.incoming.removeNode(c.Id)
			s.ll.Remove(c.elem)
			delete(s.clients, c.Id)
			s.cond.Broadcast()
		} else {
			break
		}
	}
}

func (s *server) Share(ctx context.Context, req *Request) (*Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		log.Infof("%d stopped...", s.store.meta.Id)
		return nil, errServerClosed
	}
	if req.NodeMeta.Id == 0 {
		log.Error("Receive a rpc call from a uninitialized node, drop it.")
		return nil, errUnInitialized
	}

	if ok := s.incoming.hasNode(req.NodeMeta.Id); !ok {
		s.cleanup()
		s.incoming.setMaxSize(s.maxPeers())

		if s.incoming.hasSpace() {
			log.Infof("Shared(%d): Shared(%d) connected.", s.store.meta.Id, req.NodeMeta.Id)

			s.incoming.addNode(req.NodeMeta.Id)
			c := &clientInfo{
				NodeMeta: req.NodeMeta,
				recentActiveTime: time.Now().UnixNano(),
			}
			c.elem = s.ll.PushFront(c)
			s.clients[req.NodeMeta.Id] = c
		} else {
			log.Infof("Shared(%d): Refused connection from %d since it's full.", s.store.meta.Id, req.NodeMeta.Id)
			incomingSet := s.incoming.asSlice()
			alterId := incomingSet[rand.Intn(len(incomingSet))]
			return &Response{
				NodeId: s.store.meta.Id,
				AlterNode: s.clients[alterId].NodeMeta,
			}, nil
		}
	} else {
		c := s.clients[req.NodeMeta.Id]
		c.recentActiveTime = time.Now().UnixNano()
		s.ll.MoveToFront(c.elem)
	}

	resp := &Response{
		NodeId: s.store.meta.Id,
	}

	if req.Delta != nil {
		s.received += len(req.Delta)
		freshCount, err := s.store.combine(req.Delta)
		if err != nil {
			log.Warnf("Shared(%d): Failed to fully combine infos from %d: %v", s.store.meta.Id, req.NodeMeta.Id, err)
		}
		log.Infof("Shared(%d): Updated infos(fresh: %d)", s.store.meta.Id, freshCount)
		resp.Nodes = s.store.getNodes()
		return resp, nil
	}

	for {
		resp.Delta = s.store.delta(req.Nodes)
		if len(resp.Delta) > 0 {
			s.sent += len(resp.Delta)
			resp.Nodes = s.store.getNodes()
			return resp, nil
		}
		// Wait until we have infos to reply.
		s.cond.Wait()
	}
}

func (s *server) infoSent() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent
}

func (s *server) infoReceived() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.received
}

func (s *server) maxPeers() int {
	var cnt int
	err := s.store.visitInfos(func(key string, _ *Info) error {
		if strings.HasPrefix(key, KeyNodeIdPrefix) {
			cnt++
		}
		return nil
	})
	if err != nil {
		// This should never happen.
		panic(err)
	}
	peers := int(math.Ceil(math.Exp(math.Log(float64(cnt)) / float64(MaxHops-1))))
	if peers < MinPeers {
		return MinPeers
	}
	return peers
}

func (s *server) start(worker *util.Worker) {
	updateCallback := func(_ string, _ globalpb.Value) {
		s.cond.Broadcast()
	}
	unregister := s.store.registerCallback(".*", updateCallback)

	log.Infof("SharedServer(%d) started.", s.store.meta.Id)
	worker.RunWorker(func() {
		select {
		case <-worker.ShouldStop():
			s.stop(unregister)
			return
		}
	})
}

func (s *server) stop(unregister func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	unregister()
	s.cond.Broadcast()
	log.Infof("SharedServer(%d) stopped.", s.store.meta.Id)
}
