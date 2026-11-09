package goravel

import (
	"context"
	"time"

	"gitee.com/charsen/moo-monitor-go/capture"
)

// lifecycleCapture is shared only by the queue and scheduler's safe side channels.
type lifecycleCapture struct {
	collector   *capture.Collector
	diagnostics *Handler
	timeout     time.Duration
}

func newLifecycleCapture(collector *capture.Collector, timeout time.Duration, onError OnError) (*lifecycleCapture, error) {
	if collector == nil || timeout < 0 {
		return nil, capture.ErrCaptureInput
	}
	if timeout == 0 {
		timeout = 100 * time.Millisecond
	}
	return &lifecycleCapture{collector: collector, diagnostics: &Handler{onError: onError}, timeout: timeout}, nil
}
func (o *lifecycleCapture) diagnose(ctx context.Context, stage string, failure error) {
	if ctx == nil {
		ctx = context.Background()
	}
	internal, cancel := context.WithTimeout(capture.InternalContext(ctx), o.timeout)
	defer cancel()
	o.diagnostics.diagnose(internal, Diagnostic{Stage: stage, Failure: safeFailure(failure)})
}
func (o *lifecycleCapture) record(ctx context.Context, event error, options capture.Options, stage string) (hash string, failure error) {
	defer func() {
		if recover() != nil {
			hash = ""
			failure = ErrAdapter
		}
		if failure != nil {
			o.diagnose(ctx, stage, failure)
		}
	}()
	if ctx == nil {
		return "", capture.ErrCaptureInput
	}
	if capture.IsInternal(ctx) {
		return "", nil
	}
	derived, cancel := context.WithTimeout(capture.InternalContext(ctx), o.timeout)
	defer cancel()
	return o.collector.Capture(derived, event, options)
}
