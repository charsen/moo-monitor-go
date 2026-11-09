package cloudsync

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/charsen/moo-monitor-go/buffer"
)

type pushFixtureClient struct {
	runtime func(context.Context, []buffer.Snapshot) (Receipt, error)
	slow    func(context.Context, []buffer.SlowSnapshot) (Receipt, error)
}

func (c pushFixtureClient) Send(ctx context.Context, b []buffer.Snapshot) (Receipt, error) {
	return c.runtime(ctx, b)
}
func (c pushFixtureClient) SendSlow(ctx context.Context, b []buffer.SlowSnapshot) (Receipt, error) {
	return c.slow(ctx, b)
}

func TestPusherContinuesTypesAndEmpty(t *testing.T) {
	runtime := makeStore(t, privateRoot(t), "push")
	add(t, runtime, 101, clock)
	add(t, runtime, 102, clock)
	slow, input, now := slowSyncStore(t)
	if _, _, err := slow.ReportSlowQuery(context.Background(), input, now); err != nil {
		t.Fatal(err)
	}
	var runtimeCalls, slowCalls, healthCalls atomic.Int64
	failHealth := errors.New("fixed health failure")
	client := pushFixtureClient{runtime: func(_ context.Context, b []buffer.Snapshot) (Receipt, error) {
		if runtimeCalls.Add(1) == 1 {
			reason := "upsert_failed"
			return Receipt{Saved: 1, Skipped: 1, Results: []Item{{Index: 0, Hash: b[0].Hash, Status: "saved"}, {Index: 1, Hash: b[1].Hash, Status: "skipped", Retryable: true, Reason: &reason}}}, nil
		}
		return Receipt{Saved: int64(len(b)), Legacy: true}, nil
	}, slow: func(_ context.Context, b []buffer.SlowSnapshot) (Receipt, error) {
		slowCalls.Add(1)
		return Receipt{Saved: int64(len(b)), Legacy: true}, nil
	}}
	p, err := NewPusher(runtime, slow, client, PushConfig{Runtimes: true, SlowSQL: true, Timeout: time.Second, AfterPush: func(context.Context) error { healthCalls.Add(1); return failHealth }})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(context.Background())
	first, err := p.Push(context.Background(), PushBoth)
	if !errors.Is(err, ErrPending) || !first.Runtimes.Failed || first.SlowSQL.Failed || first.Runtimes.Pending != 1 || first.Runtimes.Confirmed != 1 || first.SlowSQL.Confirmed != 1 || !errors.Is(first.HealthError, failHealth) {
		t.Fatal("partial result or next type lost", first, err)
	}
	second, err := p.Push(context.Background(), PushBoth)
	if err != nil || second.Runtimes.Confirmed != 1 || second.SlowSQL.Changed != 0 {
		t.Fatal("confirmed version resent", second, err)
	}
	empty, err := p.Tick(context.Background())
	if err != nil || empty.Runtimes.Changed != 0 || empty.SlowSQL.Changed != 0 || runtimeCalls.Load() != 2 || slowCalls.Load() != 1 || healthCalls.Load() != 3 {
		t.Fatal("empty round/health behavior", empty, err)
	}
}
func TestPusherMissingAndDisabled(t *testing.T) {
	client := pushFixtureClient{}
	p, err := NewPusher(nil, nil, client, PushConfig{Runtimes: true, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(context.Background())
	result, err := p.Tick(context.Background())
	if err != nil || result.Runtimes.Skipped != "source_unavailable" || result.SlowSQL.Skipped != "disabled" {
		t.Fatal("tick missing source", result, err)
	}
	if _, err = p.Push(context.Background(), PushRuntimes); !errors.Is(err, ErrPushSource) {
		t.Fatal("manual missing source accepted", err)
	}
	if r, e := p.Push(context.Background(), PushSlowSQL); e != nil || r.SlowSQL.Skipped != "disabled" {
		t.Fatal("disabled type not explicit", r, e)
	}
	if _, err = p.Push(context.Background(), "unknown"); !errors.Is(err, ErrPushConfig) {
		t.Fatal("invalid type accepted")
	}
}
func TestPusherBusyAndClose(t *testing.T) {
	store := makeStore(t, privateRoot(t), "push-busy")
	add(t, store, 103, clock)
	entered, release := make(chan struct{}), make(chan struct{})
	client := pushFixtureClient{runtime: func(ctx context.Context, _ []buffer.Snapshot) (Receipt, error) {
		close(entered)
		<-release
		<-ctx.Done()
		return Receipt{}, ctx.Err()
	}}
	p, err := NewPusher(store, nil, client, PushConfig{Runtimes: true, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, e := p.Push(context.Background(), PushRuntimes); done <- e }()
	<-entered
	other, err := NewPusher(store, nil, client, PushConfig{Runtimes: true, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close(context.Background())
	for _, owner := range []*Pusher{p, other} {
		r, e := owner.Push(context.Background(), PushRuntimes)
		if e != nil || !r.Runtimes.Busy {
			t.Fatal("same/cross instance lock lost", r, e)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err = p.Close(canceled); !errors.Is(err, context.Canceled) || p.State().Closed {
		t.Fatal("inflight close falsely complete", err)
	}
	if _, err = p.Push(context.Background(), PushRuntimes); !errors.Is(err, ErrPushClosed) {
		t.Fatal("new push during shutdown")
	}
	close(release)
	if err = <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("lifetime cancellation lost", err)
	}
	if err = p.Close(context.Background()); err != nil || !p.State().Closed {
		t.Fatal("close retry", err)
	}
	if err = p.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Snapshot(context.Background()); err != nil {
		t.Fatal("borrowed buffer closed", err)
	}
}

func TestPusherHealthAfterIntakeDeadline(t *testing.T) {
	store := makeStore(t, privateRoot(t), "push-timeout")
	add(t, store, 104, clock)
	called := false
	client := pushFixtureClient{runtime: func(ctx context.Context, _ []buffer.Snapshot) (Receipt, error) {
		<-ctx.Done()
		return Receipt{}, ctx.Err()
	}}
	p, err := NewPusher(store, nil, client, PushConfig{Runtimes: true, Timeout: time.Millisecond, AfterPush: func(ctx context.Context) error { called = true; return ctx.Err() }})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(context.Background())
	result, err := p.Push(context.Background(), PushRuntimes)
	if !errors.Is(err, context.DeadlineExceeded) || !called || result.HealthError != nil {
		t.Fatal("intake timeout canceled independent health operation", err, result.HealthError)
	}
}
