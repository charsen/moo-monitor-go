// Package goravel adapts the locked framework's public log contracts, without global hooks.
package goravel

import (
	"context"
	"errors"
	"sync"

	"gitee.com/charsen/moo-go-monitor/buffer"
	"gitee.com/charsen/moo-go-monitor/capture"
	"gitee.com/charsen/moo-go-monitor/records"
	"github.com/goravel/framework/contracts/log"
)

const EventIDKey = "moo_monitor_event_id"
const ExceptionKey = "moo_monitor_exception"
const FramesKey = "moo_monitor_frames"

var ErrDelegate = errors.New("delegate log handler failed")
var ErrAdapter = errors.New("runtime log capture adapter panicked")
var ErrCapture = errors.New("runtime log capture failed")

type Diagnostic struct {
	Stage          string
	Failure        error
	ObserverFailed bool
}
type Status struct {
	Failures uint64
	Last     Diagnostic
}
type OnError func(context.Context, Diagnostic)
type Handler struct {
	collector *capture.Collector
	delegate  log.Handler
	onError   OnError
	mu        sync.Mutex
	status    Status
	policy    *logPolicy
}

// Driver implements Logger while its embedded Handler supplies the real log handler and diagnostics.
type Driver struct{ *Handler }

func NewDriver(collector *capture.Collector, delegate log.Handler, onError OnError) (*Driver, error) {
	return NewDriverWithPolicy(collector, delegate, onError, nil)
}
func NewDriverWithPolicy(collector *capture.Collector, delegate log.Handler, onError OnError, input *LogPolicy) (*Driver, error) {
	if collector == nil {
		return nil, capture.ErrCaptureInput
	}
	policy, err := copyLogPolicy(input)
	if err != nil {
		return nil, err
	}
	return &Driver{&Handler{collector: collector, delegate: delegate, onError: onError, policy: policy}}, nil
}
func (d *Driver) Handle(channel string) (log.Handler, error) { return d.Handler, nil }
func (h *Handler) Enabled(level log.Level) bool {
	return h.captureEnabled(level) || (h.delegate != nil && h.delegate.Enabled(level))
}

func (h *Handler) Handle(entry log.Entry) error {
	level := entry.Level()
	var native error
	if h.delegate != nil && h.delegate.Enabled(level) {
		native = h.delegate.Handle(entry)
	}
	ctx := entry.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if capture.IsInternal(ctx) {
		return native
	}
	if native != nil {
		h.diagnose(ctx, Diagnostic{Stage: "delegate", Failure: ErrDelegate})
	}
	if !h.captureEnabled(level) {
		return native
	}
	if err := h.capture(entry, ctx); err != nil {
		h.diagnose(ctx, Diagnostic{Stage: "capture", Failure: safeFailure(err)})
	}
	return native // A monitoring failure never replaces the delegate's business result.
}

func (h *Handler) capture(entry log.Entry, ctx context.Context) (failure error) {
	defer func() {
		if recover() != nil {
			failure = ErrAdapter
		}
	}()
	with := entry.With()
	event, contextError := with[ExceptionKey].(error)
	contextError = contextError && event != nil
	if !h.sourceEnabled(contextError) {
		return nil
	}
	options := capture.Options{Source: "log_message", Meta: map[string]any{"log_level": int64(entry.Level()), "log_code": entry.Code(), "log_domain": entry.Domain()}}
	if request, ok, err := copiedHTTPRequest(ctx); ok {
		options.Request = request
		options.Payload = copiedHTTPPayload(ctx)
		if err != nil {
			h.diagnose(ctx, Diagnostic{Stage: "http-identity", Failure: safeFailure(err)})
		}
	}
	options.Scope, options.EventID = capture.EventFromContext(ctx)
	if id, ok := with[EventIDKey].(string); ok {
		options.EventID = id
	}
	if frames, ok := with[FramesKey].([]records.Frame); ok {
		options.Frames = append([]records.Frame{}, frames[:min(len(frames), 20)]...)
	} else {
		options.Frames = capture.CallerFrames(1, []string{"github.com/goravel/framework/log.", "log/slog.", "github.com/samber/slog-multi."})
	}
	internal := capture.InternalContext(ctx)
	if contextError {
		options.Source = "log_context"
		_, failure = h.collector.Capture(internal, event, options)
	} else {
		_, failure = h.collector.LogMessage(internal, entry.Message(), options)
	}
	// Do not inspect Request/Response/User, arbitrary With values or retain the pooled Entry.
	return failure
}

func (h *Handler) diagnose(ctx context.Context, diagnostic Diagnostic) {
	internal := capture.InternalContext(ctx)
	if h.onError != nil {
		func() {
			defer func() {
				if recover() != nil {
					diagnostic.ObserverFailed = true
				}
			}()
			h.onError(internal, diagnostic)
		}()
	}
	h.mu.Lock()
	h.status.Failures++
	h.status.Last = diagnostic
	h.mu.Unlock()
}
func (h *Handler) Status() Status { h.mu.Lock(); defer h.mu.Unlock(); return h.status }

// Only static categories cross the diagnostic boundary, never a replaceable Recorder's cause.
func safeFailure(failure error) (safe error) {
	safe = ErrCapture
	defer func() {
		if recover() != nil {
			safe = ErrCapture
		}
	}()
	known := []error{context.Canceled, context.DeadlineExceeded, buffer.ErrOutcomeUnknown, capture.ErrEventUncertain, capture.ErrScopeClosed, capture.ErrScopeFull, capture.ErrEventIdentity, capture.ErrCaptureInput, capture.ErrExtraction, ErrAdapter}
	queue := []error{failure}
	for inspected := 0; len(queue) > 0 && inspected < 8; inspected++ {
		current := queue[0]
		queue = queue[1:]
		if current == nil {
			continue
		}
		for _, category := range known {
			if current == category {
				safe = errors.Join(safe, category)
			}
		}
		if single, ok := current.(interface{ Unwrap() error }); ok {
			queue = append(queue, single.Unwrap())
		} else if multiple, ok := current.(interface{ Unwrap() []error }); ok {
			children := multiple.Unwrap()
			queue = append(queue, children[:min(len(children), 4)]...)
		}
	}
	return safe
}
