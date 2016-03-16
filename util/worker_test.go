package util

import (
	"testing"
	"time"
	"sync"
)

func TestWorker(t *testing.T) {
	running := make(chan struct{})
	closing := make(chan bool)

	worker := NewWorker()
	stopper := worker.ShouldStop()

	worker.AddCloser(func() {
		closing <- true
	})
	worker.RunWorker(func() {
		<-running
	})
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func(){
		select {
		case <-running:
			t.Fatal("running can't return at this time")
		case <- time.After(5 * time.Millisecond):
		}
		worker.RunTask(func() {
			close(running)
			wg.Done()
		})
	}()
	wg.Wait()
	go func() {
		worker.Stop()

		if !worker.IsStopped() {
			t.Fatal("Worker isn't stop.")
		}
	}()

	select {
	case <-stopper:
	case <-time.After(5 * time.Millisecond):
		t.Fatal("stopper should be closed")
	}

	select {
	case <- closing:
		close(closing)
	case <-time.After(5 * time.Millisecond):
		t.Fatal("Closers don't work.")
	}
}

