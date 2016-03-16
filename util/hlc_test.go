package util

import (
	"testing"
	"time"
	"sync"
	"math/rand"
	pb "continuedb/globalpb"
)

type TestPhysicalClock struct {
	mu sync.RWMutex
	nanos int64
}

func NewTestPhysicalClock(nanos int64) *TestPhysicalClock {
	return &TestPhysicalClock{nanos: nanos}
}

func (tpc *TestPhysicalClock) Increase(t int64) {
	tpc.mu.Lock()
	tpc.nanos += t
	tpc.mu.Unlock()
}

func (tpc *TestPhysicalClock) Decrease(t int64) {
	tpc.mu.Lock()
	tpc.nanos -= t
	tpc.mu.Unlock()
}

func (tpc *TestPhysicalClock) Now() int64 {
	tpc.mu.RLock()
	defer tpc.mu.RUnlock()
	return tpc.nanos
}

func TestHLC(t *testing.T) {
	tpc := NewTestPhysicalClock(UnixNano())
	hlc := NewClock(tpc.Now, time.Duration(30))

	// test PhysicalNow
	for i := 0; i < 10; i++ {
		if hlc.PhysicalNow() != tpc.Now() {
			t.Fatalf("Want: %d, got: %d", tpc.Now(), hlc.PhysicalNow())
		}
		tpc.Increase(rand.Int63n(20))
	}

	// test Now
	var ts1, ts2 pb.Timestamp
	ts1, ts2 = hlc.Now(), hlc.Now()
	if ts1.WallTime != ts2.WallTime ||
		ts1.LogicalTime > ts2.LogicalTime {
		t.Fatalf("ts1.wall should == ts2.wall && ts1.logical should < ts2.logical.")
	}
	tpc.Increase(rand.Int63n(20))
	ts1 = hlc.Now()
	tpc.Increase(rand.Int63n(20))
	ts2 = hlc.Now()
	if ts1.WallTime >= ts2.WallTime ||
		ts1.LogicalTime != ts2.LogicalTime {
		t.Fatalf("ts1.wall should < ts2.wall && ts1.logical should == ts2.logical.")
	}


	// test Update
	// when rt.timestamp == lc.timestamp
	var rt pb.Timestamp
	rt = hlc.Now()
	if tn := hlc.Update(&rt); tn.WallTime != rt.WallTime || tn.LogicalTime <= rt.LogicalTime {
		t.Fatalf("Update should have no effect this way.")
	}
	// when rt.timestamp > lc.timestamp
	tpc.Increase(10)
	rt = hlc.Now()
	tpc.Decrease(10)
	if tn := hlc.Update(&rt); tn.WallTime != rt.WallTime || tn.LogicalTime <= rt.LogicalTime {
		t.Fatalf("Update should be accept with remote.")
	}
	tpc.Increase(20)
	if tn := hlc.Update(&rt); tn.WallTime != tpc.Now() || tn.LogicalTime != 0 {
		t.Fatalf("Update should accept the local physical clock.")
	}
}

