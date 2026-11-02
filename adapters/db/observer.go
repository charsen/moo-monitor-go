// Package dbcapture observes the original DB façade without replacing its query builder.
package dbcapture

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"strings"
	"sync"
	"time"

	gormcapture "gitee.com/charsen/moo-go-monitor/adapters/gorm"
	"gitee.com/charsen/moo-go-monitor/capture"
	"gitee.com/charsen/moo-go-monitor/slowquery"
	"gorm.io/gorm"
)

var ErrConfiguration = errors.New("invalid DB observation configuration")

type Diagnostic struct {
	Kind           string
	ObserverFailed bool
}
type Status struct {
	Ready, Stopping, Closed      bool
	InFlight                     int
	Captured, Filtered, Failures uint64
	Last                         Diagnostic
}
type Config struct {
	Enabled        bool
	Recorder       gormcapture.Recorder
	Clock          func() time.Time
	Callsite       func() (string, int)
	Request        func(context.Context) (slowquery.Request, error)
	CaptureTimeout time.Duration
	OnError        func(context.Context, Diagnostic)
}
type Owner struct {
	config  Config
	mu      sync.Mutex
	status  Status
	drained chan struct{}
}

func newOwner(config Config) *Owner {
	owner := &Owner{config: config}
	if !config.Enabled {
		return owner
	}
	if config.Recorder == nil || config.CaptureTimeout < 0 {
		owner.status.Failures = 1
		owner.status.Last = Diagnostic{Kind: "configuration"}
		return owner
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.Callsite == nil {
		config.Callsite = dbCaller
	}
	if config.CaptureTimeout == 0 {
		config.CaptureTimeout = 100 * time.Millisecond
	}
	owner.config = config
	owner.status.Ready = true
	return owner
}
func (o *Owner) Status() Status { o.mu.Lock(); defer o.mu.Unlock(); return o.status }
func (o *Owner) Close(ctx context.Context) error {
	if ctx == nil {
		return ErrConfiguration
	}
	o.mu.Lock()
	o.status.Ready = false
	o.status.Stopping = true
	if o.status.InFlight == 0 {
		o.status.Closed = true
		o.mu.Unlock()
		return nil
	}
	drained := o.drained
	o.mu.Unlock()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (o *Owner) start(ctx context.Context) (when time.Time, sampled bool) {
	defer func() {
		if recover() != nil {
			when = time.Time{}
			sampled = false
			o.notify(ctx, "clock")
		}
	}()
	o.mu.Lock()
	ready := o.status.Ready
	o.mu.Unlock()
	if !ready {
		return time.Time{}, false
	}
	if ctx == nil || ctx.Err() != nil || capture.IsInternal(ctx) {
		return time.Time{}, false
	}
	return o.config.Clock(), true
}
func (o *Owner) complete(ctx context.Context, dialect gorm.Dialector, connection, raw string, args []any, start time.Time, sampled bool, native error) {
	entered := false
	failure := ""
	defer func() {
		if recover() != nil {
			failure = "panic"
		}
		if entered {
			o.mu.Lock()
			o.status.InFlight--
			if o.status.InFlight == 0 {
				close(o.drained)
				if o.status.Stopping {
					o.status.Closed = true
				}
			}
			o.mu.Unlock()
		}
		if failure != "" {
			o.notify(ctx, failure)
		}
	}()
	if !sampled || (native != nil && !errors.Is(native, sql.ErrNoRows)) {
		return
	}
	o.mu.Lock()
	if !o.status.Ready {
		o.mu.Unlock()
		return
	}
	if o.status.InFlight == 0 {
		o.drained = make(chan struct{})
	}
	o.status.InFlight++
	entered = true
	o.mu.Unlock()
	now := o.config.Clock()
	elapsed := now.Sub(start)
	if elapsed < 0 {
		failure = "clock"
		return
	}
	input, err := gormcapture.PrepareSQL(dialect, raw, args)
	if err != nil {
		failure = "input"
		return
	}
	captureCtx, cancel := context.WithTimeout(capture.InternalContext(ctx), o.config.CaptureTimeout)
	defer cancel()
	input.Request = slowquery.Request{Method: "CLI"}
	if o.config.Request != nil {
		input.Request, err = o.config.Request(captureCtx)
		if err != nil {
			failure = "request"
			return
		}
	}
	input.File, input.Line = o.config.Callsite()
	input.TookMillis = float64(elapsed.Nanoseconds()) / 1e6
	input.Connection = &connection
	_, accepted, err := o.config.Recorder.ReportSlowQuery(captureCtx, input, now)
	if err != nil {
		failure = "capture"
		return
	}
	o.mu.Lock()
	if accepted {
		o.status.Captured++
	} else {
		o.status.Filtered++
	}
	o.mu.Unlock()
}
func (o *Owner) notify(ctx context.Context, kind string) {
	d := Diagnostic{Kind: kind}
	if o.config.OnError != nil {
		func() {
			defer func() {
				if recover() != nil {
					d.ObserverFailed = true
				}
			}()
			o.config.OnError(capture.InternalContext(ctx), d)
		}()
	}
	o.mu.Lock()
	o.status.Failures++
	o.status.Last = d
	o.mu.Unlock()
}
func dbCaller() (string, int) {
	var pcs [48]uintptr
	n := runtime.Callers(2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		skip := false
		for _, prefix := range []string{"gitee.com/charsen/moo-go-monitor/", "github.com/goravel/framework/", "gorm.io/", "database/sql.", "runtime.", "testing."} {
			if strings.HasPrefix(frame.Function, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			return frame.File, frame.Line
		}
		if !more {
			return "", 0
		}
	}
}
