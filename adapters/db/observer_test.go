package dbcapture

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gitee.com/charsen/moo-monitor-go/slowquery"
	"github.com/goravel/sqlite"
)

type recorderFunc func(context.Context, slowquery.Input, time.Time) (string, bool, error)

func (f recorderFunc) ReportSlowQuery(c context.Context, i slowquery.Input, n time.Time) (string, bool, error) {
	return f(c, i, n)
}

func TestOwnerCloseCaptureAndFailure(t *testing.T) {
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	owner := newOwner(Config{Enabled: true, Recorder: recorderFunc(func(context.Context, slowquery.Input, time.Time) (string, bool, error) {
		close(entered)
		<-release
		return "", true, nil
	})})
	start, sampled := owner.start(context.Background())
	go func() {
		defer close(finished)
		owner.complete(context.Background(), sqlite.Open(":memory:"), "fixture", "SELECT 1", nil, start, sampled, nil)
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if !errors.Is(owner.Close(ctx), context.DeadlineExceeded) {
		t.Fatal("close must wait capture")
	}
	if s := owner.Status(); !s.Stopping || s.Closed || s.InFlight != 1 {
		t.Fatal(s)
	}
	close(release)
	<-finished
	if err := owner.Close(context.Background()); err != nil || !owner.Status().Closed {
		t.Fatal(err, owner.Status())
	}
	// SQL can have started before Close, but is not owned or waited on by this adapter.
	late := newOwner(Config{Enabled: true, Recorder: recorderFunc(func(context.Context, slowquery.Input, time.Time) (string, bool, error) {
		t.Fatal("late capture")
		return "", false, nil
	})})
	start, sampled = late.start(context.Background())
	if err := late.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	late.complete(context.Background(), sqlite.Open(":memory:"), "fixture", "SELECT 1", nil, start, sampled, nil)
	if late.Status().Captured != 0 {
		t.Fatal(late.Status())
	}
}

func TestOwnerSafeDiagnosticsAndNativeError(t *testing.T) {
	var calls int
	owner := newOwner(Config{Enabled: true, Recorder: recorderFunc(func(context.Context, slowquery.Input, time.Time) (string, bool, error) {
		calls++
		return "", false, errors.New("fixed-secret-token")
	}), OnError: func(_ context.Context, d Diagnostic) {
		if strings.Contains(d.Kind, "secret") {
			t.Fatal(d)
		}
		panic("fixed-observer-token")
	}})
	start, sampled := owner.start(context.Background())
	native := errors.New("original-sql-error")
	owner.complete(context.Background(), sqlite.Open(":memory:"), "fixture", "SELECT 1", nil, start, sampled, native)
	if calls != 0 || owner.Status().Failures != 0 {
		t.Fatal("native error was sampled")
	}
	owner.complete(context.Background(), sqlite.Open(":memory:"), "fixture", "SELECT 1", nil, start, sampled, nil)
	s := owner.Status()
	if calls != 1 || s.Last.Kind != "capture" || !s.Last.ObserverFailed || s.InFlight != 0 {
		t.Fatal(s)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	invalid := newOwner(Config{Enabled: true})
	if invalid.Status().Ready || invalid.Status().Last.Kind != "configuration" {
		t.Fatal(invalid.Status())
	}
}
