package goravel_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	adapter "gitee.com/charsen/moo-monitor-go/adapters/goravel"
	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/capture"
	"gitee.com/charsen/moo-monitor-go/records"
	"github.com/goravel/framework/contracts/log"
	frameworklog "github.com/goravel/framework/log"
)

var clock = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
var fixedFrames = []records.Frame{{File: "app/logger.go", Line: 7, Function: "fixture.log"}}

func setup(t *testing.T) (*capture.Collector, *buffer.Buffer) {
	t.Helper()
	root := t.TempDir()
	os.Chmod(root, 0700)
	builder, err := records.New(records.Config{MaskKeys: []string{"token"}})
	if err != nil {
		t.Fatal(err)
	}
	cap := 0
	store, err := buffer.New(buffer.Config{Root: root, DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	collector, err := capture.New(store, capture.Config{Env: "testing", Project: "fixture", GoravelVersion: "1.18.0", Clock: func() time.Time { return clock }})
	if err != nil {
		t.Fatal(err)
	}
	return collector, store
}

type delegate struct {
	mu       sync.Mutex
	messages []string
	failure  error
}

func (*delegate) Enabled(log.Level) bool { return true }
func (d *delegate) Handle(entry log.Entry) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.messages = append(d.messages, entry.Message())
	return d.failure
}
func writer(t *testing.T, ctx context.Context, driver *adapter.Driver) log.Writer {
	t.Helper()
	handler, err := driver.Handle("logging.channels.monitor")
	if err != nil {
		t.Fatal(err)
	}
	return frameworklog.NewWriter(ctx, slog.New(frameworklog.HandlerToSlogHandler(handler)))
}
func TestWriterCaptureAndDelegate(t *testing.T) {
	collector, store := setup(t)
	native := &delegate{}
	driver, err := adapter.NewDriver(collector, native, nil)
	if err != nil {
		t.Fatal(err)
	}
	logger := writer(t, context.Background(), driver)
	logger.Info("ordinary info")
	if count, err := store.Count(context.Background(), "all"); err != nil || count != 0 {
		t.Fatal("Info captured", err)
	}
	logger.With(map[string]any{adapter.FramesKey: fixedFrames, "unknown": trap{}}).Error("token=fake log")
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 1 {
		t.Fatal(err)
	}
	if snapshots[0].Record.Exception.Message != "token=*** log" || snapshots[0].Record.Exception.Class != "LoggedErrorMessage" || snapshots[0].Record.Meta["source"] != "log_message" {
		t.Fatal("string log semantics")
	}
	if len(native.messages) != 2 || native.messages[1] != "token=fake log" {
		t.Fatal("native logs changed")
	}
	if driver.Status().Failures != 0 {
		t.Fatal("unexpected diagnostic")
	}
}

type trap struct{}

func (trap) String() string { panic("unknown With object must not be read") }
func TestWriterEventIdentity(t *testing.T) {
	collector, store := setup(t)
	scope, _ := capture.NewEventScope(3)
	defer scope.Close()
	event := errors.New("fixed event")
	if _, err := collector.Capture(context.Background(), event, capture.Options{Scope: scope, EventID: "one", Source: "reportable", Frames: fixedFrames}); err != nil {
		t.Fatal(err)
	}
	driver, _ := adapter.NewDriver(collector, &delegate{}, nil)
	logger := writer(t, capture.WithEvent(context.Background(), scope, ""), driver)
	logger.With(map[string]any{adapter.EventIDKey: "one", adapter.ExceptionKey: event, adapter.FramesKey: fixedFrames}).Error("business log")
	logger.With(map[string]any{adapter.EventIDKey: "one", adapter.FramesKey: fixedFrames}).Error("string with same identity")
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 1 || snapshots[0].Record.Count != 1 || snapshots[0].Record.Meta["source"] != "log_context" {
		t.Fatal("cross-source duplicate", err)
	}
	logger.With(map[string]any{adapter.EventIDKey: "two", adapter.ExceptionKey: event, adapter.FramesKey: fixedFrames}).Error("other event id")
	snapshots, err = store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 1 || snapshots[0].Record.Count != 2 {
		t.Fatal("different identity not counted", err)
	}
}
func TestWriterFailuresAndReentry(t *testing.T) {
	collector, store := setup(t)
	native := &delegate{}
	callbacks := 0
	var driver *adapter.Driver
	driver, _ = adapter.NewDriver(collector, native, func(ctx context.Context, diagnostic adapter.Diagnostic) {
		callbacks++
		writer(t, ctx, driver).Error("internal diagnostic")
		panic("fixed observer panic")
	})
	store.Close()
	writer(t, context.Background(), driver).Error("original business message")
	if callbacks != 1 || driver.Status().Failures != 1 || !driver.Status().Last.ObserverFailed {
		t.Fatal("diagnostic recursion/panic not observed", callbacks, driver.Status())
	}
	if len(native.messages) != 2 || native.messages[0] != "original business message" || native.messages[1] != "internal diagnostic" {
		t.Fatal("delegate not preserved")
	}
	// Direct Handle returns the native error unchanged; capture failure stays out of the business return.
	failure := errors.New("native delegate failure")
	native.failure = failure
	handler, _ := driver.Handle("channel")
	record := slog.NewRecord(clock, slog.LevelError, "fixed", 0)
	entry := frameworklog.FromSlogRecord(record)
	if err := handler.Handle(entry); !errors.Is(err, failure) {
		t.Fatal("native failure cause lost", err)
	}
}
func TestWriterCancellationAndConcurrentEvents(t *testing.T) {
	collector, store := setup(t)
	driver, _ := adapter.NewDriver(collector, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	writer(t, ctx, driver).Error("cancelled capture")
	if driver.Status().Failures != 1 || !errors.Is(driver.Status().Last.Failure, context.Canceled) {
		t.Fatal("cancellation not observable")
	}
	var group sync.WaitGroup
	for i := 0; i < 6; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			writer(t, context.Background(), driver).With(map[string]any{adapter.FramesKey: fixedFrames}).Error("same message different events")
		}()
	}
	group.Wait()
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 1 || snapshots[0].Record.Count != 6 {
		t.Fatal("global busy suppression", err)
	}
}

type failingRecorder struct{ failure error }

func (r failingRecorder) Record(context.Context, records.Snapshot, time.Time) (string, error) {
	return "", r.failure
}
func (r failingRecorder) TagSource(context.Context, string, string, map[string]any, time.Time) (bool, error) {
	return false, r.failure
}
func TestWriterSafeDiagnosticBoundary(t *testing.T) {
	raw := errors.New("fixed-fake-token in replaceable Recorder error")
	collector, err := capture.New(failingRecorder{failure: raw}, capture.Config{Clock: func() time.Time { return clock }})
	if err != nil {
		t.Fatal(err)
	}
	native := &delegate{}
	observed := ""
	driver, _ := adapter.NewDriver(collector, native, func(ctx context.Context, diagnostic adapter.Diagnostic) { observed = fmt.Sprint(diagnostic) })
	writer(t, context.Background(), driver).Error("original business message")
	if strings.Contains(observed, "fixed-fake-token") || strings.Contains(fmt.Sprint(driver.Status()), "fixed-fake-token") || !errors.Is(driver.Status().Last.Failure, adapter.ErrCapture) {
		t.Fatal("raw diagnostic escaped")
	}
	if len(native.messages) != 1 || native.messages[0] != "original business message" {
		t.Fatal("delegate changed")
	}
	// Explicit capture keeps its returned cause; only the automatic diagnostic surface is sanitized.
	if _, err := collector.Capture(context.Background(), errors.New("event"), capture.Options{Frames: fixedFrames}); !errors.Is(err, raw) {
		t.Fatal("explicit cause lost", err)
	}
}
