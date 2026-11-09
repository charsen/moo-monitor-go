package goravel_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	adapter "gitee.com/charsen/moo-monitor-go/adapters/goravel"
	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/capture"
	contractsqueue "github.com/goravel/framework/contracts/queue"
	frameworkerrors "github.com/goravel/framework/errors"
	frameworkjson "github.com/goravel/framework/foundation/json"
	mocklog "github.com/goravel/framework/mocks/log"
	mockqueue "github.com/goravel/framework/mocks/queue"
	frameworkqueue "github.com/goravel/framework/queue"
	"github.com/stretchr/testify/mock"
)

type memoryQueue struct {
	items          chan contractsqueue.ReservedJob
	pops, receives atomic.Int64
}

func (*memoryQueue) Driver() string { return "memory-fixture" }
func (d *memoryQueue) Push(task contractsqueue.Task, _ string) error {
	d.items <- &reservedFixture{task: task, done: make(chan struct{})}
	return nil
}
func (d *memoryQueue) Pop(string) (contractsqueue.ReservedJob, error) {
	d.pops.Add(1)
	select {
	case job := <-d.items:
		return job, nil
	default:
		return nil, frameworkerrors.QueueDriverNoJobFound
	}
}

type receiveQueue struct{ *memoryQueue }

func (d *receiveQueue) Receive(ctx context.Context, _ string, count int) ([]contractsqueue.ReservedJob, error) {
	d.receives.Add(1)
	select {
	case first := <-d.items:
		jobs := []contractsqueue.ReservedJob{first}
		for len(jobs) < count {
			select {
			case next := <-d.items:
				jobs = append(jobs, next)
			default:
				return jobs, nil
			}
		}
		return jobs, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type reservedFixture struct {
	task    contractsqueue.Task
	done    chan struct{}
	failure error
	once    sync.Once
}

func (r *reservedFixture) Task() contractsqueue.Task { return r.task }
func (r *reservedFixture) Delete() error             { r.once.Do(func() { close(r.done) }); return r.failure }

type queueJob struct {
	name   string
	handle func(...any) error
}

func (j *queueJob) Signature() string        { return j.name }
func (j *queueJob) Handle(args ...any) error { return j.handle(args...) }

type retryQueueJob struct {
	*queueJob
	policy func(error, int) (bool, time.Duration)
}

func (j *retryQueueJob) ShouldRetry(err error, attempt int) (bool, time.Duration) {
	return j.policy(err, attempt)
}
func queueDeps(t *testing.T, backend contractsqueue.Driver, jobs ...contractsqueue.Job) adapter.WorkerDependencies {
	t.Helper()
	config := mockqueue.NewConfig(t)
	config.On("Driver", "fixture").Return(contractsqueue.DriverCustom).Once()
	config.On("Via", "fixture").Return(backend).Once()
	config.On("Debug").Return(false)
	config.On("FailedDatabase").Return("").Maybe()
	config.On("FailedTable").Return("").Maybe()
	log := mocklog.NewLog(t)
	log.On("Error", mock.Anything).Return().Maybe()
	storer := frameworkqueue.NewJobStorer()
	storer.Register(jobs)
	return adapter.WorkerDependencies{Config: config, Jobs: storer, JSON: frameworkjson.New(), Log: log}
}
func runQueue(t *testing.T, deps adapter.WorkerDependencies, config adapter.WorkerConfig, reservations ...*reservedFixture) *adapter.QueueWorker {
	t.Helper()
	worker, err := adapter.NewQueueWorker(deps, config)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- worker.Run() }()
	for _, job := range reservations {
		select {
		case <-job.done:
		case <-time.After(5 * time.Second):
			t.Fatal("real Worker did not complete reservation")
		}
	}
	if err := worker.Shutdown(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Worker did not shut down")
	}
	return worker
}
func totalRuntime(t *testing.T, store *buffer.Buffer) int64 {
	t.Helper()
	all, err := store.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	for _, v := range all {
		count += v.Record.Count
	}
	return count
}
func TestQueueTerminalDecisions(t *testing.T) {
	for _, tries := range []int{0, 1, 3} {
		t.Run(fmt.Sprint(tries), func(t *testing.T) {
			collector, store := setup(t)
			var calls atomic.Int64
			job := &queueJob{name: "fixture-final", handle: func(...any) error { calls.Add(1); return errors.New("terminal fixture") }}
			reserved := &reservedFixture{task: contractsqueue.Task{UUID: "same-id", ChainJob: contractsqueue.ChainJob{Job: job}}, done: make(chan struct{})}
			backend := &memoryQueue{items: make(chan contractsqueue.ReservedJob, 1)}
			backend.items <- reserved
			worker := runQueue(t, queueDeps(t, backend, job), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, Tries: tries, Collector: collector}, reserved)
			if calls.Load() != int64(max(tries, 1)) || totalRuntime(t, store) != 1 || worker.Status().Failures != 0 {
				t.Fatal("terminal decision changed", calls.Load(), worker.Status())
			}
			all, _ := store.Snapshot(context.Background())
			meta := all[0].Record.Meta
			if meta["source"] != "queue_failed" || meta["job_name"] != "fixture-final" || fmt.Sprint(meta["attempts"]) != fmt.Sprint(max(tries, 1)) {
				t.Fatal("terminal metadata missing", meta)
			}
		})
	}
}
func TestQueuePolicySuccessAndChain(t *testing.T) {
	collector, store := setup(t)
	var calls, policies atomic.Int64
	base := &queueJob{name: "retry-success", handle: func(...any) error {
		if calls.Add(1) < 3 {
			return errors.New("temporary fixture")
		}
		return nil
	}}
	job := &retryQueueJob{queueJob: base, policy: func(err error, attempt int) (bool, time.Duration) { policies.Add(1); return attempt < 3, 0 }}
	var second, unreachable atomic.Int64
	fail := &queueJob{name: "chain-fail", handle: func(...any) error { second.Add(1); return errors.New("chain terminal") }}
	never := &queueJob{name: "chain-never", handle: func(...any) error { unreachable.Add(1); return nil }}
	reserved := &reservedFixture{task: contractsqueue.Task{UUID: "chain-id", ChainJob: contractsqueue.ChainJob{Job: job}, Chain: []contractsqueue.ChainJob{{Job: fail}, {Job: never}}}, done: make(chan struct{})}
	backend := &memoryQueue{items: make(chan contractsqueue.ReservedJob, 1)}
	backend.items <- reserved
	runQueue(t, queueDeps(t, backend, job, fail, never), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, Tries: 2, Collector: collector}, reserved)
	if calls.Load() != 3 || policies.Load() != 2 || second.Load() != 2 || unreachable.Load() != 0 || totalRuntime(t, store) != 1 {
		t.Fatal("policy/chain changed")
	}
	if _, ok := reserved.task.Job.(contractsqueue.JobWithShouldRetry); !ok {
		t.Fatal("fixture original policy lost")
	}
	if _, ok := reserved.task.Chain[0].Job.(contractsqueue.JobWithShouldRetry); ok {
		t.Fatal("original chain slice was mutated")
	}
}
func TestQueueRetryThenSuccessDoesNotCapture(t *testing.T) {
	collector, store := setup(t)
	var calls atomic.Int64
	job := &queueJob{name: "eventual-success", handle: func(...any) error {
		if calls.Add(1) < 3 {
			return errors.New("temporary")
		}
		return nil
	}}
	reserved := &reservedFixture{task: contractsqueue.Task{ChainJob: contractsqueue.ChainJob{Job: job}}, done: make(chan struct{})}
	backend := &memoryQueue{items: make(chan contractsqueue.ReservedJob, 1)}
	backend.items <- reserved
	runQueue(t, queueDeps(t, backend, job), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, Tries: 3, Collector: collector}, reserved)
	if calls.Load() != 3 || totalRuntime(t, store) != 0 {
		t.Fatal("retry attempt was misreported as terminal")
	}
}
func TestQueueReceiveConcurrentAndFailureIsolation(t *testing.T) {
	collector, store := setup(t)
	var calls, inflight, peak atomic.Int64
	gate := make(chan struct{})
	job := &queueJob{name: "concurrent-final", handle: func(...any) error {
		active := inflight.Add(1)
		defer inflight.Add(-1)
		for current := peak.Load(); active > current; current = peak.Load() {
			if peak.CompareAndSwap(current, active) {
				break
			}
		}
		if calls.Add(1) == 4 {
			close(gate)
		}
		select {
		case <-gate:
		case <-time.After(30 * time.Second):
			return errors.New("concurrency barrier timed out")
		}
		return errors.New("same terminal")
	}}
	native := &memoryQueue{items: make(chan contractsqueue.ReservedJob, 8)}
	backend := &receiveQueue{native}
	var reserved []*reservedFixture
	for i := 0; i < 8; i++ {
		r := &reservedFixture{task: contractsqueue.Task{UUID: "same-delivery-id", ChainJob: contractsqueue.ChainJob{Job: job}}, done: make(chan struct{}), failure: errors.New("delete fixture")}
		reserved = append(reserved, r)
		native.items <- r
	}
	// The default capture budget is a 100ms production isolation guard. Under
	// -race and a loaded machine it can expire mid-write, silently dropping events;
	// that would test the machine's latency, not the worker's semantics.
	runQueue(t, queueDeps(t, backend, job), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 4, Tries: 1, Collector: collector, CaptureTimeout: 5 * time.Second}, reserved...)
	if got := calls.Load(); got != 8 {
		t.Fatalf("handled %d jobs, want 8", got)
	}
	if got := peak.Load(); got < 4 {
		t.Fatalf("peak concurrency %d, want at least 4", got)
	}
	if got := totalRuntime(t, store); got != 8 {
		t.Fatalf("captured %d runtime events, want 8", got)
	}
	if native.receives.Load() == 0 {
		t.Fatal("worker never used the receive path")
	}
	if got := native.pops.Load(); got != 0 {
		t.Fatalf("worker used pop %d times, want 0", got)
	}
	// Capacity one cannot refuse a second chain task; success itself never consumes an event ID.
}
func TestQueueCaptureFailureDoesNotChangeRetries(t *testing.T) {
	collector, store := setup(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	job := &queueJob{name: "closed-monitor", handle: func(...any) error { calls.Add(1); return errors.New("native business failure") }}
	reserved := &reservedFixture{task: contractsqueue.Task{ChainJob: contractsqueue.ChainJob{Job: job}}, done: make(chan struct{})}
	backend := &memoryQueue{items: make(chan contractsqueue.ReservedJob, 1)}
	backend.items <- reserved
	worker := runQueue(t, queueDeps(t, backend, job), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, Tries: 2, Collector: collector, OnError: func(context.Context, adapter.Diagnostic) { panic("fixed observer") }}, reserved)
	if calls.Load() != 2 || worker.Status().Failures == 0 || !worker.Status().Last.ObserverFailed {
		t.Fatal("monitor failure changed terminal processing")
	}
}

// Compile-time checks use the actual public contracts rather than a private worker patch.
var _ contractsqueue.Worker = (*adapter.QueueWorker)(nil)

func TestQueueOriginalPolicyTerminal(t *testing.T) {
	collector, store := setup(t)
	var calls, policies atomic.Int64
	job := &retryQueueJob{queueJob: &queueJob{name: "policy-terminal", handle: func(...any) error { calls.Add(1); return errors.New("policy terminal") }}, policy: func(error, int) (bool, time.Duration) { policies.Add(1); return false, 77 * time.Millisecond }}
	reserved := &reservedFixture{task: contractsqueue.Task{ChainJob: contractsqueue.ChainJob{Job: job}}, done: make(chan struct{})}
	backend := &memoryQueue{items: make(chan contractsqueue.ReservedJob, 1)}
	backend.items <- reserved
	runQueue(t, queueDeps(t, backend, job), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, Tries: 99, MaxEvents: 1, Collector: collector}, reserved)
	if calls.Load() != 1 || policies.Load() != 1 || totalRuntime(t, store) != 1 {
		t.Fatal("original policy was replaced or called twice")
	}
}
func TestQueueTerminalBeforePayloadSerializationFailure(t *testing.T) {
	collector, store := setup(t)
	job := &queueJob{name: "bad-native-payload", handle: func(...any) error { return errors.New("terminal before serialization") }}
	reserved := &reservedFixture{task: contractsqueue.Task{ChainJob: contractsqueue.ChainJob{Job: job, Args: []contractsqueue.Arg{{Type: "unsupported-native", Value: make(chan int)}}}}, done: make(chan struct{})}
	backend := &memoryQueue{items: make(chan contractsqueue.ReservedJob, 1)}
	backend.items <- reserved
	runQueue(t, queueDeps(t, backend, job), adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, Tries: 1, Collector: collector}, reserved)
	if totalRuntime(t, store) != 1 {
		t.Fatal("actual terminal decision lost when native payload serialization failed")
	}
}
func TestQueueNilBackendRejected(t *testing.T) {
	collector, _ := setup(t)
	config := mockqueue.NewConfig(t)
	config.On("Driver", "fixture").Return(contractsqueue.DriverCustom).Once()
	config.On("Via", "fixture").Return(func() (contractsqueue.Driver, error) { return nil, nil }).Once()
	logger := mocklog.NewLog(t)
	_, err := adapter.NewQueueWorker(adapter.WorkerDependencies{Config: config, Jobs: frameworkqueue.NewJobStorer(), JSON: frameworkjson.New(), Log: logger}, adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, Collector: collector})
	if !errors.Is(err, capture.ErrCaptureInput) {
		t.Fatal("nil backend was accepted")
	}
}
