package capture

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/charsen/moo-go-monitor/buffer"
	"gitee.com/charsen/moo-go-monitor/records"
)

var now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.FixedZone("fixture", 8*3600))
var frames = []records.Frame{{File: "app/capture.go", Line: 17, Function: "fixture.capture"}}

func setup(t *testing.T) (*Collector, *buffer.Buffer) {
	t.Helper()
	root := t.TempDir()
	os.Chmod(root, 0700)
	builder, err := records.New(records.Config{MaskKeys: []string{"token", "password"}})
	if err != nil {
		t.Fatal(err)
	}
	cap := 0
	store, err := buffer.New(buffer.Config{Root: root, DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	collector, err := New(store, Config{Env: "testing", Project: "fixture", GoVersion: "fixture-go", Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return collector, store
}

type nonComparable struct{ values []string }

func (nonComparable) Error() string { return "token=fake 中文" }
func TestCaptureScope(t *testing.T) {
	collector, store := setup(t)
	scope, err := NewEventScope(2)
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	event := nonComparable{[]string{"not a map key"}}
	options := Options{Scope: scope, EventID: "one", Source: "reportable", Frames: frames}
	hash, err := collector.Capture(context.Background(), event, options)
	if err != nil {
		t.Fatal(err)
	}
	options.Source = "queue_failed"
	if duplicate, err := collector.Capture(context.Background(), event, options); err != nil || duplicate != hash {
		t.Fatal(err)
	}
	record, err := store.Get(context.Background(), hash)
	if err != nil || record.Count != 1 || record.Meta["source"] != "queue_failed" || record.Exception.Message != "token=*** 中文" {
		t.Fatal("same-ID tag", err)
	}
	options.Source = "log_context"
	collector.Capture(context.Background(), event, options)
	options.EventID = "two"
	options.Source = "reportable"
	if _, err := collector.Capture(context.Background(), event, options); err != nil {
		t.Fatal(err)
	}
	record, _ = store.Get(context.Background(), hash)
	if record.Count != 2 {
		t.Fatal("different ID did not count")
	}
	options.EventID = "three"
	if _, err := collector.Capture(context.Background(), event, options); !errors.Is(err, ErrScopeFull) {
		t.Fatal("unbounded scope", err)
	}
	scope.Close()
	options.EventID = "one"
	if _, err := collector.Capture(context.Background(), event, options); !errors.Is(err, ErrScopeClosed) {
		t.Fatal("closed scope reused", err)
	}
	if len(scope.entries) != 0 {
		t.Fatal("scope did not release identities")
	}
}

type unknownRecorder struct {
	*buffer.Buffer
	calls atomic.Int64
}

func (r *unknownRecorder) Record(ctx context.Context, s records.Snapshot, now time.Time) (string, error) {
	r.calls.Add(1)
	hash, err := r.Buffer.Record(ctx, s, now)
	if err != nil {
		return hash, err
	}
	return hash, buffer.ErrOutcomeUnknown
}
func TestCaptureUnknown(t *testing.T) {
	_, store := setup(t)
	recorder := &unknownRecorder{Buffer: store}
	collector, _ := New(recorder, Config{Clock: func() time.Time { return now }})
	scope, _ := NewEventScope(1)
	defer scope.Close()
	options := Options{Scope: scope, EventID: "uncertain", Frames: frames}
	hash, err := collector.Capture(context.Background(), errors.New("fixed error"), options)
	if !errors.Is(err, buffer.ErrOutcomeUnknown) {
		t.Fatal(err)
	}
	if _, err = collector.Capture(context.Background(), errors.New("fixed error"), options); !errors.Is(err, ErrEventUncertain) {
		t.Fatal("unknown retried", err)
	}
	record, _ := store.Get(context.Background(), hash)
	if recorder.calls.Load() != 1 || record.Count != 1 {
		t.Fatal("unknown double counted")
	}
}

type cycle struct{}

func (*cycle) Error() string   { return "cycle" }
func (c *cycle) Unwrap() error { return c }

type badError struct{}

func (badError) Error() string { panic("fixed fake secret must not appear") }
func TestCaptureErrorExtraction(t *testing.T) {
	collector, store := setup(t)
	wrapped := fmt.Errorf("outer: %w", errors.Join(errors.New("password=fake"), errors.New("second"), errors.New("third"), errors.New("fourth")))
	hash, err := collector.Capture(context.Background(), wrapped, Options{Frames: frames})
	if err != nil {
		t.Fatal(err)
	}
	record, _ := store.Get(context.Background(), hash)
	if len(record.Exception.Previous) != 3 || record.Context.Go != "fixture-go" || record.Exception.File != "app/capture.go" {
		t.Fatal("explicit capture snapshot")
	}
	hash, err = collector.Capture(context.Background(), &cycle{}, Options{Frames: frames})
	if err != nil {
		t.Fatal(err)
	}
	record, _ = store.Get(context.Background(), hash)
	if len(record.Exception.Previous) != 3 {
		t.Fatal("cycle was not bounded")
	}
	if _, err = collector.Capture(context.Background(), badError{}, Options{Frames: frames}); !errors.Is(err, ErrExtraction) {
		t.Fatal("panic extraction not observable", err)
	}
	hash, err = collector.LogMessage(context.Background(), string([]byte{'x', 0xff}), Options{Frames: frames})
	if err != nil {
		t.Fatal(err)
	}
	record, _ = store.Get(context.Background(), hash)
	if record.Exception.Message != string([]byte{'x', 0xff}) || record.Exception.Class != "LoggedErrorMessage" {
		t.Fatal("Builder byte semantics changed")
	}
}

type slowRecorder struct {
	*buffer.Buffer
	entered, release chan struct{}
}

func (r *slowRecorder) Record(ctx context.Context, s records.Snapshot, now time.Time) (string, error) {
	close(r.entered)
	select {
	case <-r.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return r.Buffer.Record(ctx, s, now)
}
func TestCaptureCancellation(t *testing.T) {
	_, store := setup(t)
	recorder := &slowRecorder{Buffer: store, entered: make(chan struct{}), release: make(chan struct{})}
	collector, _ := New(recorder, Config{Clock: func() time.Time { return now }})
	scope, _ := NewEventScope(2)
	defer scope.Close()
	done := make(chan error, 1)
	go func() {
		_, err := collector.Capture(context.Background(), errors.New("fixed"), Options{Scope: scope, EventID: "same", Frames: frames})
		done <- err
	}()
	<-recorder.entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := collector.Capture(ctx, errors.New("fixed"), Options{Scope: scope, EventID: "same", Frames: frames}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	close(recorder.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := collector.Capture(ctx, errors.New("fixed"), Options{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	store.Close()
	if _, err := collector.Capture(context.Background(), errors.New("fixed"), Options{Frames: frames}); err == nil {
		t.Fatal("closed store captured")
	}
}
