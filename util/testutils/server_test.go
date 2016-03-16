package testutils

import (
	"testing"
	"time"
)

func TestTestServer(t *testing.T) {
	ts := NewTestServer(true)
	ts.GetDialOpts()
	ts.Start()
	time.Sleep(10 * time.Second)
	ts.Stop()
}
