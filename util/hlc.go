package util

import (
	"sync"
	pb "continuedb/globalpb"
	"time"
	log "github.com/Sirupsen/logrus"
)

// Clock is the hlc clock representation in the whole database.
type Clock struct {
	physicalClock func() int64

	mu sync.Mutex
	pb.Timestamp
	maxOffset time.Duration
}

func UnixNano() int64 {
	return time.Now().UnixNano()
}

// NewClock creates a hlc Clock with the given physicalClock and maxOffset.
// Normally the physicalClock is the `UnixNano` function.
func NewClock(physicalClock func() int64, maxOffset time.Duration) *Clock {
	return &Clock{
		physicalClock: physicalClock,
		maxOffset: maxOffset,
	}
}

// timestampLocked returns the timestamp of the Clock.
// The caller of this method should have the lock of the Clock.
func (c *Clock) timestampLocked() pb.Timestamp {
	return c.Timestamp
}

// PhysicalNow returns the current physical time of the hlc depends.
func (c *Clock) PhysicalNow() int64 {
	return c.physicalClock()
}

// Now returns the current hlc time.
func (c *Clock) Now() pb.Timestamp {
	c.mu.Lock()
	defer c.mu.Unlock()

	wallTime := c.physicalClock()
	if wallTime > c.WallTime {
		c.WallTime = wallTime
		c.LogicalTime = 0
	} else {
		c.LogicalTime++
	}

	return c.timestampLocked()
}

// Update updates the Clock with the timestamp of the received message and
// returns the updated timestamp.
func (c *Clock) Update(rt *pb.Timestamp) pb.Timestamp {
	c.mu.Lock()
	defer c.mu.Unlock()

	wallTime := c.physicalClock()
	if wallTime > rt.WallTime && wallTime > c.WallTime {
		c.WallTime = wallTime
		c.LogicalTime = 0
	} else if rt.WallTime > c.WallTime {
		if rt.WallTime - wallTime > c.maxOffset.Nanoseconds() {
			log.Errorf("The offset between the wall time of the remote and local exceeds the maximum allowed:" +
				" (allowed: %d, real: %d)", c.maxOffset.Nanoseconds(), rt.WallTime - wallTime)
		}
		c.WallTime = rt.WallTime
		c.LogicalTime = rt.LogicalTime + 1
	} else if c.WallTime > rt.WallTime {
		c.LogicalTime++
	} else {
		if rt.LogicalTime > c.LogicalTime {
			c.LogicalTime = rt.LogicalTime
		}
		c.LogicalTime++
	}

	return c.timestampLocked()
}