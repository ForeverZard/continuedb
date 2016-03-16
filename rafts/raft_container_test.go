package rafts

import (
	pb "continuedb/rafts/pb"
	"continuedb/util/testutils"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"continuedb/globalpb"
)

const NUM_OF_RAFTS_PER_CONTAINER = 3

// containerList contains all nodes metas. It has to be set manually.
var containerList = []globalpb.NodeMeta{
	{1, "172.28.179.6", testutils.Port},
	{2, "172.16.151.138", testutils.Port},
}

func getContainerMeta(ip string) *globalpb.NodeMeta {
	for _, m := range containerList {
		if m.Host == ip {
			return &m
		}
	}
	return nil
}

type testRouter struct {
	mu    sync.Mutex
	store map[uint64]*globalpb.NodeMeta
}

func newTestRouter() *testRouter {
	return &testRouter{
		store: make(map[uint64]*globalpb.NodeMeta),
	}
}

func (tr *testRouter) Lookup(id uint64) *globalpb.NodeMeta {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.store[id]
}

func (tr *testRouter) SetRoute(rid uint64, meta *globalpb.NodeMeta) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.store[rid] = meta
}

var ts *testutils.TestServer

func init() {
	ts = testutils.NewTestServer(false)
}

type RaftRPCRegister func(*grpc.Server, pb.RaftServer)

func (rr RaftRPCRegister) Register(s *grpc.Server, i interface{}) {
	rr(s, i.(pb.RaftServer))
}

func TestRaftContainer(t *testing.T) {
	stopc := make(chan struct{})

	tr := newTestRouter()
	container := NewRaftContainer(NewContainerMeta(1, ts.Host, ts.Port), tr, ts.S)

	ts.Start()

	container.Start()

	m := getContainerMeta(ts.Host)
	if m == nil {
		panic("Can't get container meta of host" + ts.Host)
	}

	var peers []uint64
	for ci, m := range containerList {
		for i := 0; i < NUM_OF_RAFTS_PER_CONTAINER; i++ {
			peers = append(peers, iconcat(m.Id, uint64(i)))
			tr.SetRoute(iconcat(m.Id, uint64(i)), &containerList[ci])
		}
	}

	var rafts []*Raft
	for i := 0; i < NUM_OF_RAFTS_PER_CONTAINER; i++ {
		cfg := &RaftConfig{
			Id:      uint64(iconcat(m.Id, uint64(i))),
			Storage: NewInMemStorage(nil),
			Inc:     make(chan pb.Message),
			Outc:    make(chan pb.Message),
		}
		r, err := NewRaft(cfg)
		if err != nil {
			panic(err)
		}

		container.Add(r)

		r.Start(peers)
		rafts = append(rafts, r)
	}

	go func() {
		for {
			select {
			case <-stopc:
				return
			default:
				time.Sleep(50 * time.Millisecond)
				Tick()
			}
		}
	}()

	go func() {
		time.Sleep(20000 * time.Millisecond)
		close(stopc)
	}()

	<-stopc
	for _, r := range rafts {
		container.Remove(r.id)
	}
	container.Stop()
	ts.Stop()
}
