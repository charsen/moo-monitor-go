package cloudsync

import (
	"context"
	"errors"
	"sync"
	"time"

	"gitee.com/charsen/moo-go-monitor/buffer"
)

type PushTarget string

const (
	PushBoth     PushTarget = "both"
	PushRuntimes PushTarget = "runtimes"
	PushSlowSQL  PushTarget = "slow_sql"
)

var ErrPushConfig = errors.New("invalid Cloud push configuration")
var ErrPushSource = errors.New("selected Cloud push source unavailable")
var ErrPushClosed = errors.New("Cloud pusher is stopping or closed")

type PushClient interface {
	Client
	SlowClient
}
type PushConfig struct {
	Runtimes, SlowSQL bool
	Timeout           time.Duration
	BatchSize         *int
	// AfterPush is optional. A later health adapter supplies the real operation;
	// its failure is independent from intake success and never invents an ACK.
	AfterPush func(context.Context) error
}
type TypePushResult struct {
	Result
	Skipped string
	Failed  bool
}
type PushResult struct {
	Runtimes, SlowSQL TypePushResult
	HealthError       error
}
type PushState struct {
	Stopping, Closed bool
	InFlight         int
}
type Pusher struct {
	runtime  *Syncer
	slow     *SlowSyncer
	config   PushConfig
	mu       sync.Mutex
	state    PushState
	idle     chan struct{}
	lifetime context.Context
	cancel   context.CancelFunc
}

func NewPusher(runtime *buffer.Buffer, slow *buffer.SlowBuffer, client PushClient, config PushConfig) (*Pusher, error) {
	if client == nil || (!config.Runtimes && !config.SlowSQL) || config.Timeout <= 0 {
		return nil, ErrPushConfig
	}
	if config.BatchSize != nil && *config.BatchSize < 1 {
		return nil, ErrPushConfig
	}
	lifetime, cancel := context.WithCancel(context.Background())
	p := &Pusher{config: config, lifetime: lifetime, cancel: cancel, idle: make(chan struct{})}
	close(p.idle)
	if runtime != nil {
		value, err := New(runtime, client, Config{BatchSize: config.BatchSize})
		if err != nil {
			cancel()
			return nil, err
		}
		p.runtime = value
	}
	if slow != nil {
		value, err := NewSlow(slow, client, Config{BatchSize: config.BatchSize})
		if err != nil {
			if p.runtime != nil {
				p.runtime.Close()
			}
			cancel()
			return nil, err
		}
		p.slow = value
	}
	return p, nil
}
func (p *Pusher) Push(ctx context.Context, target PushTarget) (PushResult, error) {
	return p.PushWithOptions(ctx, target, SyncOptions{Mode: Incremental})
}
func (p *Pusher) PushWithOptions(ctx context.Context, target PushTarget, options SyncOptions) (PushResult, error) {
	return p.run(ctx, target, true, options)
}
func (p *Pusher) Tick(ctx context.Context) (PushResult, error) {
	return p.run(ctx, PushBoth, false, SyncOptions{Mode: Incremental})
}
func (p *Pusher) run(ctx context.Context, target PushTarget, strict bool, options SyncOptions) (PushResult, error) {
	result := PushResult{Runtimes: TypePushResult{Skipped: "not_selected"}, SlowSQL: TypePushResult{Skipped: "not_selected"}}
	if !validSelectionMode(options.Mode) || ctx == nil || (target != PushBoth && target != PushRuntimes && target != PushSlowSQL) {
		return result, ErrPushConfig
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	round, leave, err := p.enterOperation(ctx)
	if err != nil {
		return result, err
	}
	defer leave()
	operation, cancel := context.WithTimeout(round, p.config.Timeout)
	defer cancel()
	var failure error
	if target == PushBoth || target == PushRuntimes {
		switch {
		case !p.config.Runtimes:
			result.Runtimes.Skipped = "disabled"
		case p.runtime == nil:
			result.Runtimes.Skipped = "source_unavailable"
			if strict {
				result.Runtimes.Failed = true
				failure = errors.Join(failure, ErrPushSource)
			}
		default:
			result.Runtimes.Skipped = ""
			value, err := p.runtime.SyncWithOptions(operation, options)
			result.Runtimes.Result = value
			result.Runtimes.Failed = err != nil
			failure = errors.Join(failure, err)
		}
	}
	if target == PushBoth || target == PushSlowSQL {
		switch {
		case !p.config.SlowSQL:
			result.SlowSQL.Skipped = "disabled"
		case p.slow == nil:
			result.SlowSQL.Skipped = "source_unavailable"
			if strict {
				result.SlowSQL.Failed = true
				failure = errors.Join(failure, ErrPushSource)
			}
		default:
			result.SlowSQL.Skipped = ""
			value, err := p.slow.SyncWithOptions(operation, options)
			result.SlowSQL.Result = value
			result.SlowSQL.Failed = err != nil
			failure = errors.Join(failure, err)
		}
	}
	if p.config.AfterPush != nil {
		result.HealthError = p.config.AfterPush(round)
	}
	return result, failure
}

// enterOperation makes Push and read-only Stats participate in the same owner lifetime.
func (p *Pusher) enterOperation(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		return nil, nil, ErrPushConfig
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	p.mu.Lock()
	if p.state.Stopping || p.state.Closed {
		p.mu.Unlock()
		return nil, nil, ErrPushClosed
	}
	if p.state.InFlight == 0 {
		p.idle = make(chan struct{})
	}
	p.state.InFlight++
	p.mu.Unlock()
	operation, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(p.lifetime, cancel)
	leave := func() {
		stop()
		cancel()
		p.mu.Lock()
		p.state.InFlight--
		if p.state.InFlight == 0 {
			close(p.idle)
		}
		p.mu.Unlock()
	}
	return operation, leave, nil
}
func (p *Pusher) State() PushState { p.mu.Lock(); defer p.mu.Unlock(); return p.state }
func (p *Pusher) Close(ctx context.Context) error {
	if ctx == nil {
		return ErrPushConfig
	}
	p.mu.Lock()
	if p.state.Closed {
		p.mu.Unlock()
		return nil
	}
	p.state.Stopping = true
	p.cancel()
	idle := p.idle
	p.mu.Unlock()
	select {
	case <-idle:
	case <-ctx.Done():
		return ctx.Err()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state.Closed {
		return nil
	}
	var failure error
	if p.runtime != nil {
		failure = errors.Join(failure, p.runtime.Close())
	}
	if p.slow != nil {
		failure = errors.Join(failure, p.slow.Close())
	}
	if failure == nil {
		p.state.Closed = true
	}
	return failure
}
