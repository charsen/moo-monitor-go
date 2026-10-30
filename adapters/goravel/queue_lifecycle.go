package goravel

import (
	"context"
	"errors"
	"sync"

	queue "github.com/goravel/framework/contracts/queue"
)

var ErrWorkerAlreadyRun = errors.New("queue worker may only run once")

type WorkerState struct{ Running, Stopping, Closed bool }

// Pop must admit every native worker goroutine before an external Shutdown can
// wait on its WaitGroup. Receive has a single native receiver and needs one entry.
type pollStart struct {
	mu              sync.Mutex
	entered, target int
	started         chan struct{}
}

func newPollStart(target int) *pollStart {
	return &pollStart{target: target, started: make(chan struct{})}
}
func (s *pollStart) enter() {
	s.mu.Lock()
	if s.entered < s.target {
		s.entered++
		if s.entered == s.target {
			close(s.started)
		}
	}
	s.mu.Unlock()
	<-s.started
}

type workerLifecycle struct {
	native                                queue.Worker
	started                               <-chan struct{}
	done, shutdownDone                    chan struct{}
	mu                                    sync.Mutex
	requested, stopping, closed, finished bool
	runErr, shutdownErr                   error
	shutdownOnce                          sync.Once
}

func newWorkerLifecycle(native queue.Worker, started <-chan struct{}) *workerLifecycle {
	return &workerLifecycle{native: native, started: started, done: make(chan struct{}), shutdownDone: make(chan struct{})}
}
func (w *workerLifecycle) run() error {
	w.mu.Lock()
	if w.requested {
		w.mu.Unlock()
		return ErrWorkerAlreadyRun
	}
	if w.stopping {
		w.requested = true
		w.mu.Unlock()
		return nil
	}
	w.requested = true
	w.mu.Unlock()
	defer func() { w.mu.Lock(); w.finished = true; w.mu.Unlock(); close(w.done) }()
	err := w.native.Run()
	w.mu.Lock()
	w.runErr = err
	w.mu.Unlock()
	return err
}
func (w *workerLifecycle) stop(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	w.mu.Lock()
	if !w.requested {
		// No native Run has entered, so never close its unstarted failed-job channel.
		if !w.closed {
			w.stopping = true
			w.closed = true
			w.finished = true
			close(w.done)
			close(w.shutdownDone)
		}
		w.mu.Unlock()
		return nil
	}
	if w.closed {
		err := w.shutdownErr
		w.mu.Unlock()
		return err
	}
	w.stopping = true
	w.mu.Unlock()
	select {
	case <-w.started:
		w.shutdownOnce.Do(func() {
			go func() {
				err := w.native.Shutdown()
				<-w.done
				w.mu.Lock()
				w.shutdownErr = err
				w.closed = err == nil
				w.mu.Unlock()
				close(w.shutdownDone)
			}()
		})
	case <-w.done:
		// A sync/non-consuming worker can return without ever entering a poll.
		w.shutdownOnce.Do(func() {
			w.mu.Lock()
			w.shutdownErr = w.runErr
			w.closed = w.runErr == nil
			w.mu.Unlock()
			close(w.shutdownDone)
		})
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-w.shutdownDone:
		w.mu.Lock()
		err := w.shutdownErr
		w.mu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (w *workerLifecycle) state() WorkerState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return WorkerState{Running: w.requested && !w.finished, Stopping: w.stopping, Closed: w.closed}
}
