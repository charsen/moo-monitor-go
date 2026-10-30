package goravel

import (
	"context"
	"strconv"
	"time"

	"gitee.com/charsen/moo-go-monitor/capture"
	"github.com/goravel/framework/contracts/cache"
	"github.com/goravel/framework/contracts/database/db"
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/contracts/log"
	queue "github.com/goravel/framework/contracts/queue"
	frameworkqueue "github.com/goravel/framework/queue"
)

type WorkerDependencies struct {
	Config queue.Config
	Cache  cache.Cache
	DB     db.DB
	Jobs   queue.JobStorer
	JSON   foundation.Json
	Log    log.Log
}
type WorkerConfig struct {
	Connection, Queue            string
	Concurrent, Tries, MaxEvents int
	Collector                    *capture.Collector
	Context                      context.Context
	CaptureTimeout               time.Duration
	OnError                      OnError
	DisableCapture               bool
}
type QueueWorker struct {
	lifecycle   *workerLifecycle
	observation *lifecycleCapture
}

// NewQueueWorker constructs the normal backend once and gives that same instance
// to a real Goravel Worker. Borrowed database/cache/logger resources are not closed.
func NewQueueWorker(deps WorkerDependencies, config WorkerConfig) (*QueueWorker, error) {
	var observation *lifecycleCapture
	var err error
	if !config.DisableCapture {
		observation, err = newLifecycleCapture(config.Collector, config.CaptureTimeout, config.OnError)
		if err != nil {
			return nil, err
		}
	}
	if deps.Config == nil || deps.Jobs == nil || deps.JSON == nil || deps.Log == nil {
		return nil, capture.ErrCaptureInput
	}
	if config.Context == nil {
		config.Context = context.Background()
	}
	if config.Connection == "" {
		config.Connection = deps.Config.DefaultConnection()
	}
	if config.Queue == "" {
		config.Queue = deps.Config.GetString("queue.connections."+config.Connection+".queue", "default")
	}
	if config.Concurrent == 0 {
		config.Concurrent = deps.Config.GetInt("queue.connections."+config.Connection+".concurrent", 1)
	}
	if config.Concurrent < 1 {
		return nil, capture.ErrCaptureInput
	}
	if !config.DisableCapture && config.MaxEvents == 0 {
		config.MaxEvents = 64
	}
	if !config.DisableCapture && (config.MaxEvents < 1 || config.MaxEvents > 4096) {
		return nil, capture.ErrCaptureInput
	}
	backend, err := frameworkqueue.NewDriverCreator(deps.Config, deps.Cache, deps.DB, deps.Jobs, deps.JSON, deps.Log).Create(config.Connection)
	if err != nil {
		return nil, err
	}
	if backend == nil {
		return nil, capture.ErrCaptureInput
	}
	gate := newPollStart(config.Concurrent)
	wrapper := &queueDriver{backend: backend, observation: observation, config: config, start: gate}
	var driver queue.Driver = wrapper
	if receiver, ok := backend.(queue.DriverWithReceive); ok {
		gate.target = 1
		driver = &queueReceiveDriver{queueDriver: wrapper, receiver: receiver}
	}
	localConfig := workerConfig{Config: deps.Config, connection: config.Connection, driver: driver}
	native, err := frameworkqueue.NewWorker(localConfig, deps.Cache, deps.DB, deps.Jobs, deps.JSON, deps.Log, config.Connection, config.Queue, config.Concurrent, config.Tries)
	if err != nil {
		return nil, err
	}
	return &QueueWorker{lifecycle: newWorkerLifecycle(native, gate.started), observation: observation}, nil
}
func (w *QueueWorker) Run() error                                { return w.lifecycle.run() }
func (w *QueueWorker) Shutdown() error                           { return w.lifecycle.stop(context.Background()) }
func (w *QueueWorker) ShutdownContext(ctx context.Context) error { return w.lifecycle.stop(ctx) }
func (w *QueueWorker) Started() <-chan struct{}                  { return w.lifecycle.started }
func (w *QueueWorker) State() WorkerState                        { return w.lifecycle.state() }
func (w *QueueWorker) Status() Status {
	if w.observation == nil {
		return Status{}
	}
	return w.observation.diagnostics.Status()
}

type workerConfig struct {
	queue.Config
	connection string
	driver     queue.Driver
}

func (c workerConfig) Driver(connection string) string {
	if connection == c.connection {
		return queue.DriverCustom
	}
	return c.Config.Driver(connection)
}
func (c workerConfig) Via(connection string) any {
	if connection == c.connection {
		return c.driver
	}
	return c.Config.Via(connection)
}

type queueDriver struct {
	backend     queue.Driver
	observation *lifecycleCapture
	config      WorkerConfig
	start       *pollStart
}

func (d *queueDriver) Driver() string { return d.backend.Driver() }

// Push delegates untouched; only already-reserved tasks are observed.
func (d *queueDriver) Push(task queue.Task, name string) error { return d.backend.Push(task, name) }
func (d *queueDriver) Pop(name string) (queue.ReservedJob, error) {
	d.start.enter()
	reserved, err := d.backend.Pop(name)
	if err != nil || reserved == nil {
		return reserved, err
	}
	return d.wrap(reserved), nil
}
func (d *queueDriver) wrap(reserved queue.ReservedJob) queue.ReservedJob {
	if d.observation == nil {
		return reserved
	}
	scope, _ := capture.NewEventScope(d.config.MaxEvents) // Constructor validated the bound.
	return &queueReservation{ReservedJob: reserved, driver: d, scope: scope}
}

type queueReceiveDriver struct {
	*queueDriver
	receiver queue.DriverWithReceive
}

func (d *queueReceiveDriver) Receive(ctx context.Context, name string, count int) ([]queue.ReservedJob, error) {
	d.start.enter()
	jobs, err := d.receiver.Receive(ctx, name, count)
	if err != nil {
		return jobs, err
	}
	result := make([]queue.ReservedJob, len(jobs))
	for i, job := range jobs {
		if job != nil {
			result[i] = d.wrap(job)
		}
	}
	return result, nil
}

type queueReservation struct {
	queue.ReservedJob
	driver *queueDriver
	scope  *capture.EventScope
}

func (r *queueReservation) Task() queue.Task {
	task := r.ReservedJob.Task()
	task.Args = append([]queue.Arg(nil), task.Args...)
	task.Job = r.job(task.Job, 0)
	task.Chain = append([]queue.ChainJob(nil), task.Chain...)
	for i := range task.Chain {
		task.Chain[i].Args = append([]queue.Arg(nil), task.Chain[i].Args...)
		task.Chain[i].Job = r.job(task.Chain[i].Job, i+1)
	}
	return task
}
func (r *queueReservation) job(original queue.Job, index int) queue.Job {
	if original == nil {
		return nil
	}
	return &terminalJob{Job: original, driver: r.driver, scope: r.scope, id: "queue:" + strconv.Itoa(index)}
}
func (r *queueReservation) Delete() error { defer r.scope.Close(); return r.ReservedJob.Delete() }

type terminalJob struct {
	queue.Job
	driver *queueDriver
	scope  *capture.EventScope
	id     string
}

func (j *terminalJob) ShouldRetry(failure error, attempt int) (bool, time.Duration) {
	retry, delay := attempt < j.driver.config.Tries, time.Duration(0)
	if policy, ok := j.Job.(queue.JobWithShouldRetry); ok {
		retry, delay = policy.ShouldRetry(failure, attempt)
	}
	if !retry {
		j.recordFinal(failure, attempt)
	}
	return retry, delay
}
func (j *terminalJob) recordFinal(failure error, attempt int) {
	// Metadata/observer failures cannot replace the policy's already-decided result.
	defer func() {
		if recover() != nil {
			j.driver.observation.diagnose(j.driver.config.Context, "queue-final", ErrAdapter)
		}
	}()
	options := capture.Options{Scope: j.scope, EventID: j.id, Source: "queue_failed", Meta: map[string]any{
		"connection": j.driver.config.Connection, "queue": j.driver.config.Queue, "job_name": j.Job.Signature(), "attempts": int64(attempt),
	}}
	_, _ = j.driver.observation.record(j.driver.config.Context, failure, options, "queue-final")
}
