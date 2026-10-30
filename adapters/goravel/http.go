package goravel

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"gitee.com/charsen/moo-go-monitor/capture"
	"gitee.com/charsen/moo-go-monitor/records"
	contractshttp "github.com/goravel/framework/contracts/http"
)

var ErrHTTPContext = errors.New("invalid HTTP capture lifecycle or event identity")

type HTTPIdentity struct{ ID, Name, Guard *string }
type Recover func(contractshttp.Context, any)
type HTTPConfig struct {
	MaxEvents int
	OnError   OnError
	Recover   Recover
	Identity  func(context.Context) (HTTPIdentity, error)
}
type HTTP struct {
	collector   *capture.Collector
	config      HTTPConfig
	diagnostics *Handler
	next        atomic.Uint64
}
type httpKey struct{}
type httpEvent struct {
	id     string
	event  error
	frames []records.Frame
}
type httpState struct {
	mu       sync.Mutex
	owner    *HTTP
	scope    *capture.EventScope
	request  records.Request
	marked   *httpEvent
	closed   bool
	identity func(context.Context) (HTTPIdentity, error)
}

func NewHTTP(collector *capture.Collector, config HTTPConfig) (*HTTP, error) {
	if collector == nil {
		return nil, capture.ErrCaptureInput
	}
	if config.MaxEvents == 0 {
		config.MaxEvents = 64
	}
	if config.MaxEvents < 1 || config.MaxEvents > 4096 {
		return nil, capture.ErrEventIdentity
	}
	return &HTTP{collector: collector, config: config, diagnostics: &Handler{onError: config.OnError}}, nil
}
func (h *HTTP) Signature() string { return "moo-monitor:http-capture" }
func (h *HTTP) Status() Status    { return h.diagnostics.Status() }

func (h *HTTP) Handle(ctx contractshttp.Context) {
	scope, _ := capture.NewEventScope(h.config.MaxEvents)
	state := &httpState{owner: h, scope: scope, identity: h.config.Identity}
	ctx.WithContext(context.WithValue(ctx.Context(), httpKey{}, state))
	// The same defer owns observation, explicit recovery and final scope cleanup.
	defer func() {
		defer func() {
			state.mu.Lock()
			state.closed = true
			state.marked = nil
			state.request = records.Request{}
			state.identity = nil
			state.mu.Unlock()
			scope.Close()
		}()
		if value := recover(); value != nil {
			if h.config.Recover == nil {
				panic(value)
			}
			h.WrapRecover(h.config.Recover)(ctx, value)
			h.observe(ctx, state)
			return
		}
		h.observe(ctx, state)
	}()
	request, err := h.requestSnapshot(ctx)
	state.request = request
	if err != nil {
		h.report(ctx.Context(), err)
	}
	ctx.Request().Next()
}

// EventContext binds only one caller-identified event, never every error in the request.
func (h *HTTP) EventContext(ctx context.Context, id string) (context.Context, error) {
	state, err := h.state(ctx, id)
	if err != nil {
		return nil, err
	}
	return capture.WithEvent(ctx, state.scope, id), nil
}

// MarkError selects the explicit terminal error for a later 5xx; it does not record it.
func (h *HTTP) MarkError(ctx context.Context, id string, event error, frames []records.Frame) error {
	if event == nil {
		return ErrHTTPContext
	}
	state, err := h.state(ctx, id)
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return ErrHTTPContext
	}
	state.marked = &httpEvent{id: id, event: event, frames: append([]records.Frame{}, frames[:min(len(frames), 20)]...)}
	return nil
}
func (h *HTTP) state(ctx context.Context, id string) (*httpState, error) {
	if ctx == nil || id == "" || len(id) > 128 || !utf8.ValidString(id) || strings.HasPrefix(id, "__moo_http:") {
		return nil, ErrHTTPContext
	}
	state, _ := ctx.Value(httpKey{}).(*httpState)
	if state == nil || state.owner != h {
		return nil, ErrHTTPContext
	}
	state.mu.Lock()
	closed := state.closed
	state.mu.Unlock()
	if closed {
		return nil, ErrHTTPContext
	}
	return state, nil
}

// WrapRecover never registers a global hook. A standalone owner supplies and closes its lifecycle.
func (h *HTTP) WrapRecover(delegate Recover) Recover {
	return func(ctx contractshttp.Context, value any) {
		// Capture is a side effect; even its failure cannot skip the owner's response callback.
		h.capturePanic(ctx, value)
		if delegate != nil {
			delegate(ctx, value)
		}
	}
}
func (h *HTTP) capturePanic(ctx contractshttp.Context, value any) {
	var err error
	defer func() {
		if recover() != nil {
			h.report(ctx.Context(), ErrAdapter)
		}
	}()
	state, _ := ctx.Context().Value(httpKey{}).(*httpState)
	options := capture.Options{Source: "reportable", Frames: capture.CallerFrames(1, nil)}
	id := fmt.Sprintf("__moo_http:panic:%d", h.next.Add(1))
	if state != nil && state.owner == h {
		options.Scope = state.scope
		options.EventID = id
		request, _, identityErr := copiedHTTPRequest(ctx.Context())
		options.Request = request
		if identityErr != nil {
			h.report(ctx.Context(), identityErr)
		}
	}
	event, ok := value.(error)
	if !ok {
		message := "HTTP panic"
		if text, yes := value.(string); yes {
			message = text
		} else if value != nil {
			message += " (" + reflect.TypeOf(value).String() + ")"
		}
		event = HTTPPanic{message}
	}
	_, err = h.collector.Capture(ctx.Context(), event, options)
	if err != nil {
		h.report(ctx.Context(), err)
	}
	if options.Scope != nil {
		state.mu.Lock()
		state.marked = &httpEvent{id: id, event: event, frames: options.Frames}
		state.mu.Unlock()
		// The original callback's logs share this exact panic identity while the scope is alive.
		ctx.WithContext(capture.WithEvent(ctx.Context(), state.scope, id))
	}
}

type HTTPPanic struct{ Message string }

func (e HTTPPanic) Error() string { return e.Message }

type HTTPStatus struct{ Status int }

func (e HTTPStatus) Error() string { return fmt.Sprintf("HTTP response status %d", e.Status) }

func (h *HTTP) observe(ctx contractshttp.Context, state *httpState) {
	defer func() {
		if recover() != nil {
			h.report(ctx.Context(), ErrAdapter)
		}
	}()
	status := ctx.Response().Origin().Status()
	if status < 500 {
		return
	}
	request, _, identityErr := copiedHTTPRequest(ctx.Context())
	if identityErr != nil {
		h.report(ctx.Context(), identityErr)
	}
	options := capture.Options{Scope: state.scope, Source: "http_5xx", Request: request, Meta: map[string]any{"http_status": int64(status)}}
	state.mu.Lock()
	marked := state.marked
	state.mu.Unlock()
	var event error = HTTPStatus{status}
	if marked != nil {
		options.EventID = marked.id
		options.Frames = marked.frames
		event = marked.event
	} else {
		options.EventID = fmt.Sprintf("__moo_http:status:%d", h.next.Add(1))
		options.Frames = []records.Frame{{File: "moo-monitor/http", Function: "HTTPStatus"}}
	}
	if _, err := h.collector.Capture(ctx.Context(), event, options); err != nil {
		h.report(ctx.Context(), err)
	}
}
func (h *HTTP) requestSnapshot(ctx contractshttp.Context) (snapshot records.Request, failure error) {
	defer func() {
		if recover() != nil {
			snapshot = records.Request{}
			failure = ErrAdapter
		}
	}()
	request := ctx.Request()
	url, ip := request.FullUrl(), request.Ip()
	snapshot = records.Request{Method: request.Method(), URL: &url, IP: &ip}
	return snapshot, nil
}
func copyText(value *string) *string {
	if value == nil {
		return nil
	}
	text := *value
	return &text
}
func (h *HTTP) report(ctx context.Context, err error) {
	h.diagnostics.diagnose(ctx, Diagnostic{Stage: "http", Failure: safeFailure(err)})
}

// copiedHTTPRequest consumes only the lifecycle's scalar snapshot, not a framework request.
func copiedHTTPRequest(ctx context.Context) (request records.Request, exists bool, failure error) {
	if ctx == nil {
		return records.Request{}, false, nil
	}
	state, _ := ctx.Value(httpKey{}).(*httpState)
	if state == nil {
		return records.Request{}, false, nil
	}
	state.mu.Lock()
	if state.closed {
		state.mu.Unlock()
		return records.Request{}, false, nil
	}
	request = state.request
	identity := state.identity
	request.URL = copyText(request.URL)
	request.IP = copyText(request.IP)
	state.mu.Unlock()
	exists = true
	if identity == nil {
		return request, true, nil
	}
	// Resolve at the event, after authentication; retain no framework request or closure over it.
	defer func() {
		if recover() != nil {
			failure = ErrAdapter
		}
	}()
	values, err := identity(ctx)
	if err != nil {
		return request, true, err
	}
	request.UserID = copyText(values.ID)
	request.UserName = copyText(values.Name)
	request.Guard = copyText(values.Guard)
	return request, true, nil
}
