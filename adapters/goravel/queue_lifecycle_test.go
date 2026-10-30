package goravel_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	adapter "gitee.com/charsen/moo-go-monitor/adapters/goravel"
	contractsqueue "github.com/goravel/framework/contracts/queue"
	frameworkerrors "github.com/goravel/framework/errors"
	frameworkqueue "github.com/goravel/framework/queue"
)

func TestQueueManagedDisabledCapture(t *testing.T) {
	var calls atomic.Int64
	job := &queueJob{name: "without-monitor", handle: func(...any) error { calls.Add(1); return errors.New("fixed failure") }}
	reserved := &reservedFixture{task: contractsqueue.Task{ChainJob: contractsqueue.ChainJob{Job: job}}, done: make(chan struct{})}
	backend := &memoryQueue{items: make(chan contractsqueue.ReservedJob, 1)}
	backend.items <- reserved
	worker := runQueue(t, queueDeps(t, backend, job), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, Tries: 2, DisableCapture: true, CaptureTimeout: -1, MaxEvents: -1}, reserved)
	if calls.Load() != 2 || worker.Status().Failures != 0 || !worker.State().Closed {
		t.Fatal("capture-disabled business or close changed")
	}
	if err := worker.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(worker.Run(), adapter.ErrWorkerAlreadyRun) {
		t.Fatal("second native Run was allowed")
	}
}
func TestQueueManagedStopBeforeRun(t *testing.T) {
	backend := &memoryQueue{items: make(chan contractsqueue.ReservedJob, 1)}
	worker, err := adapter.NewQueueWorker(queueDeps(t, backend), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 4, DisableCapture: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err = worker.Run(); err != nil {
		t.Fatal(err)
	}
	if backend.pops.Load() != 0 || !worker.State().Closed || worker.State().Running {
		t.Fatal("stopped worker entered native Run")
	}
	select {
	case <-worker.Started():
		t.Fatal("unstarted poll was claimed ready")
	default:
	}
}
func TestQueueManagedEarlyStopAndCancellation(t *testing.T) {
	for _, concurrent := range []int{1, 4, 16} {
		backend := &memoryQueue{items: make(chan contractsqueue.ReservedJob, 1)}
		worker, err := adapter.NewQueueWorker(queueDeps(t, backend), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: concurrent, DisableCapture: true})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- worker.Run() }()
		select {
		case <-worker.Started():
		case <-time.After(3 * time.Second):
			t.Fatal("initial poll barrier not released")
		}
		// Every native goroutine has passed its Add before this signal.
		if err = worker.Shutdown(); err != nil {
			t.Fatal(err)
		}
		select {
		case err = <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("early shutdown did not finish")
		}
	}
	collector, _ := setup(t)
	entered, release := make(chan struct{}), make(chan struct{})
	job := &queueJob{name: "inflight", handle: func(...any) error { close(entered); <-release; return nil }}
	reserved := &reservedFixture{task: contractsqueue.Task{ChainJob: contractsqueue.ChainJob{Job: job}}, done: make(chan struct{})}
	backend := &memoryQueue{items: make(chan contractsqueue.ReservedJob, 1)}
	backend.items <- reserved
	worker, err := adapter.NewQueueWorker(queueDeps(t, backend, job), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, Collector: collector})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- worker.Run() }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("job did not enter")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = worker.ShutdownContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation not visible", err)
	}
	if worker.State().Closed {
		t.Fatal("inflight work was claimed closed")
	}
	close(release)
	if err = worker.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if !worker.State().Closed {
		t.Fatal("retry did not complete shutdown")
	}
}

func TestQueueManagedReceiveIdle(t *testing.T) {
	backend := &receiveQueue{&memoryQueue{items: make(chan contractsqueue.ReservedJob)}}
	worker, err := adapter.NewQueueWorker(queueDeps(t, backend), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 8, DisableCapture: true})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- worker.Run() }()
	select {
	case <-worker.Started():
	case <-time.After(3 * time.Second):
		t.Fatal("receive readiness")
	}
	if err = worker.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if backend.receives.Load() != 1 || !worker.State().Closed {
		t.Fatal("native receive not canceled once")
	}
}

func TestQueueManagedSyncReturn(t *testing.T) {
	worker, err := adapter.NewQueueWorker(queueDeps(t, frameworkqueue.NewSync()), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, DisableCapture: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Run(); err != nil {
		t.Fatal(err)
	}
	if err = worker.Shutdown(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-worker.Started():
		t.Fatal("sync worker claimed poll readiness")
	default:
	}
	if !worker.State().Closed {
		t.Fatal("sync completion not closed")
	}
}

type blockingPop struct {
	*memoryQueue
	release chan struct{}
}

func (d *blockingPop) Pop(string) (contractsqueue.ReservedJob, error) {
	<-d.release
	return nil, frameworkerrors.QueueDriverNoJobFound
}
func TestQueueManagedUninterruptiblePop(t *testing.T) {
	backend := &blockingPop{memoryQueue: &memoryQueue{}, release: make(chan struct{})}
	worker, err := adapter.NewQueueWorker(queueDeps(t, backend), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, DisableCapture: true})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- worker.Run() }()
	select {
	case <-worker.Started():
	case <-time.After(3 * time.Second):
		t.Fatal("pop readiness")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err = worker.ShutdownContext(ctx); !errors.Is(err, context.DeadlineExceeded) || worker.State().Closed {
		t.Fatal("blocked original Pop claimed stopped", err)
	}
	close(backend.release)
	if err = worker.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
