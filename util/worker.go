package util

import (
	"sync"
	log "github.com/Sirupsen/logrus"
)

// Worker is used to run long time processes.
type Worker struct {
	wg sync.WaitGroup
	mu sync.Mutex
	drain *sync.Cond
	numTasks int
	closers []func()
	stopper chan struct{}
	drainer chan struct{}
	stopped chan struct{}
	draining bool
}

func NewWorker() *Worker {
	w := &Worker{
		stopper: make(chan struct{}),
		drainer: make(chan struct{}),
		stopped: make(chan struct{}),
	}
	w.drain = sync.NewCond(&w.mu)
	return w
}

func(w *Worker) AddCloser(closer func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closers = append(w.closers, closer)
}

func(w *Worker) ShouldDrain() <-chan struct{} {
	if w == nil {
		return nil
	}
	return w.drainer
}

func(w *Worker) ShouldStop() <-chan struct{} {
	if w == nil {
		return nil
	}
	return w.stopper
}

func(w *Worker) RunWorker(f func()) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		f()
	}()
}

func(w *Worker) prelude() bool {
	if w.draining {
		return false
	};
	w.mu.Lock()
	w.numTasks++
	w.mu.Unlock()
	return true
}

func(w *Worker) postlude() {
	w.mu.Lock()
	w.numTasks--
	w.drain.Broadcast()
	w.mu.Unlock()
}

func(w *Worker) RunTask(f func()) bool {
	if !w.prelude() {
		return false;
	}
	f()
	w.postlude()
	return true
}

func(w *Worker) RunTaskAsync(f func()) bool {
	if !w.prelude() {
		return false;
	}
	go func() {
		defer w.postlude()
		f()
	}()
	return true
}

func(w *Worker) Stop() {
	log.Infoln("Worker is stopping...")
	w.Drain()
	close(w.stopper)
	w.wg.Wait()
	w.mu.Lock()
	for _, f := range w.closers {
		f()
	}
	w.mu.Unlock()
	close(w.stopped)
	log.Infoln("Worker stopped.")
}

func(w *Worker) Drain() {
	if !w.draining {
		w.draining = true
		close(w.drainer)
	}
	w.mu.Lock()
	for w.numTasks > 0 {
		w.drain.Wait()
	}
	w.mu.Unlock()
}

func(w *Worker) NumTasks() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.numTasks
}

func(w *Worker) IsStopped() bool {
	select {
	case <-w.stopped:
		return true
	default:
		return false;
	}
}