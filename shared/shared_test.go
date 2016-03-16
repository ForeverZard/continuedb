package shared

import (
	"testing"
	"continuedb/globalpb"
	"strconv"
	"continuedb/util/testutils"
	"continuedb/util"
	"time"
)


func TestShared(t *testing.T) {
	var (
		peers []*globalpb.NodeMeta
		tss []*testutils.TestServer
		ss []*Shared
	)
	for i := 0; i < 3; i++ {
		meta := &globalpb.NodeMeta{uint64(i+1), "127.0.0.1", ":" + strconv.Itoa(10000 + i)}
		peers = append(peers, meta)
	}

	for i := 0; i < 3; i++ {
		ts := testutils.NewTestServer(peers[i].Host, peers[i].Port, false)
		s := NewShared(peers[i], peers, ts.S)
		tss = append(tss, ts)
		ss = append(ss, s)
	}

	worker := util.NewWorker()

	for i := 0; i < 3; i++ {
		tss[i].Start()
		ss[i].Start(worker)
	}

	select {
	case <-time.After(2 * time.Second):
	}

	worker.Stop()
}
