package shared

import (
	"continuedb/globalpb"
	"google.golang.org/grpc"
	"continuedb/util"
	"golang.org/x/net/context"
	log "github.com/Sirupsen/logrus"
	"sync"
)

type rpcType int
const (
	rpcPull rpcType = iota
	rpcPush
)

type rpcArgs struct {
	rpcType rpcType
	resp *Response
}

type client struct {
	conn        *grpc.ClientConn
	grpcClient  SharedClient
	serverMeta  *globalpb.NodeMeta
	alterMeta   *globalpb.NodeMeta
	remoteNodes map[uint64]*Node
	rpcChan     chan *rpcArgs
	closer      chan struct{}

	mu          sync.Mutex
	pushing     bool
	pushWanted  int
}

func newClient(meta *globalpb.NodeMeta) *client {
	return &client{
		serverMeta: meta,
		remoteNodes: make(map[uint64]*Node),
		rpcChan: make(chan *rpcArgs, 10),
		closer: make(chan struct{}),
	}
}

func (c *client) start(s *Shared, done chan *client, worker *util.Worker) {
	worker.RunWorker(func() {
		meta := c.serverMeta
		conn, err := grpc.Dial(meta.Host + meta.Port, grpc.WithInsecure())
		if err != nil {
			panic(err)
		}
		c.conn = conn
		c.grpcClient = NewSharedClient(conn)

		c.shared(s, worker)

		done <- c
	})
}

func (c *client) stop() {
	close(c.closer)
}

func (c *client) pull(s *Shared) {
	s.mu.Lock()
	req := &Request{
		NodeMeta: s.store.meta,
		Nodes: s.store.getNodes(),
	}
	s.mu.Unlock()

	resp, err := c.grpcClient.Share(context.Background(), req)

	if err != nil {
		c.stop()
		return
	}
	c.rpcChan <- &rpcArgs{rpcPull, resp}
}

func (c *client) push(s *Shared) {
	c.mu.Lock()
	if c.pushing {
		c.pushWanted++
		c.mu.Unlock()
		return
	} else {
		c.pushing = true
		c.pushWanted = 0
	}
	c.mu.Unlock()

	s.mu.Lock()
	req := &Request{
		NodeMeta: s.store.meta,
		Delta: s.store.delta(c.remoteNodes),
		Nodes: s.store.getNodes(),
	}
	s.mu.Unlock()

	resp, err := c.grpcClient.Share(context.Background(), req)

	if err != nil {
		log.Infof("[%d -> %d] err = %v", s.meta.Id, c.serverMeta.Id, err)
		c.stop()
		return
	}
	c.rpcChan <- &rpcArgs{rpcPush, resp}
}

func (c *client) shared(s *Shared, worker *util.Worker) {
	c.pull(s)

	updateCallback := func(_ string, _ globalpb.Value) {
		c.push(s)
	}

	defer s.RegisterCallback(".*", updateCallback)()

	for {
		select {
		case args := <-c.rpcChan:
			c.handleResponse(s, args.resp)
			if args.rpcType == rpcPull {
				c.pull(s)
			} else {
				c.mu.Lock()
				c.pushing = false
				needPush := c.pushWanted > 0
				c.mu.Unlock()
				if needPush {
					c.push(s)
				}
			}
		case <-c.closer:
			log.Infof("Shared(%d): client to %d stopped.", s.meta.Id, c.serverMeta.Id)
			return
		case <-worker.ShouldStop():
		log.Infof("Shared(%d): client to %d stopped.", s.meta.Id, c.serverMeta.Id)
			return
		}
	}
}

func (c *client) handleResponse(s *Shared, resp *Response) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if resp.Delta != nil {
		freshCount, err := s.store.combine(resp.Delta)
		if err != nil {
			log.Warnf("Shared(%d) failed to combine infos from %d: %v", s.meta.Id, resp.NodeId, err)
		}
		if freshCount > 0 {
			log.Infof("Shared(%d) updated %d infos from SharedServer(%d)", s.meta.Id, freshCount, c.serverMeta.Id)
		}
	}
	s.outgoing.addNode(resp.NodeId)
	c.remoteNodes = resp.Nodes

	if resp.AlterNode != nil {
		c.conn.Close()
		c.alterMeta = resp.AlterNode
	}
}
