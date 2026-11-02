// Package gormcapture observes explicitly installed, locked GORM SQLite/Postgres/MySQL instances.
package gormcapture

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gitee.com/charsen/moo-go-monitor/capture"
	"gitee.com/charsen/moo-go-monitor/slowquery"
	"gorm.io/gorm"
)

const pluginName = "moo-monitor:slow-query"

var ErrConfiguration = errors.New("invalid GORM monitor configuration")
var ErrUnsupported = errors.New("unsupported GORM monitor instance")
var ErrInstalled = errors.New("GORM monitor instance already has an owner")
var ErrClosed = errors.New("GORM monitor is closed")
var ErrRollback = errors.New("GORM monitor installation rollback failed")
var installation sync.Mutex

type Recorder interface {
	ReportSlowQuery(context.Context, slowquery.Input, time.Time) (string, bool, error)
}
type Failure string

const (
	FailureCapture Failure = "capture"
	FailureRequest Failure = "request"
	FailureInput   Failure = "input"
	FailureClock   Failure = "clock"
	FailurePanic   Failure = "panic"
)

type Diagnostic struct {
	Kind           Failure
	ObserverFailed bool
}
type Status struct {
	Closed                                       bool
	InFlight                                     int
	Captured, Filtered, Failures, ObserverPanics uint64
	Last                                         Diagnostic
}
type Config struct {
	Connection     string
	Recorder       Recorder
	Clock          func() time.Time
	Request        func(context.Context) (slowquery.Request, error)
	Callsite       func() (string, int)
	CaptureTimeout time.Duration
	OnError        func(context.Context, Diagnostic)
}
type Plugin struct {
	config  Config
	mu      sync.Mutex
	ready   bool
	status  Status
	drained chan struct{}
}

func New(config Config) (*Plugin, error) {
	if config.Recorder == nil || strings.TrimSpace(config.Connection) == "" || len(config.Connection) > 128 || !utf8.ValidString(config.Connection) || config.CaptureTimeout < 0 {
		return nil, ErrConfiguration
	}
	if config.CaptureTimeout == 0 {
		config.CaptureTimeout = 100 * time.Millisecond
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.Callsite == nil {
		config.Callsite = caller
	}
	return &Plugin{config: config}, nil
}
func (p *Plugin) Name() string { return pluginName }

// Install consumes an existing public implementation capability, never constructing a DB connection.
// Registration must happen before traffic. GORM callback compilation itself is not query-concurrent safe.
func Install(query any, plugin *Plugin) (failure error) {
	defer func() {
		if recover() != nil {
			failure = ErrUnsupported
		}
	}()
	if plugin == nil {
		return ErrConfiguration
	}
	capable, ok := query.(interface{ Instance() *gorm.DB })
	if !ok {
		return ErrUnsupported
	}
	db := capable.Instance()
	if db == nil || db.Config == nil || db.Dialector == nil || !supportedDialect(db.Dialector.Name()) {
		return ErrUnsupported
	}
	installation.Lock()
	defer installation.Unlock()
	plugin.mu.Lock()
	closed := plugin.status.Closed
	plugin.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if current, exists := db.Plugins[pluginName]; exists {
		if current == plugin {
			return nil
		}
		return ErrInstalled
	}
	if err := db.Use(plugin); err != nil {
		return ErrConfiguration
	}
	return nil
}
func (p *Plugin) Initialize(db *gorm.DB) (failure error) {
	if db == nil || db.Dialector == nil || !supportedDialect(db.Dialector.Name()) {
		return ErrUnsupported
	}
	p.mu.Lock()
	unavailable := p.ready || p.status.Closed
	p.mu.Unlock()
	if unavailable {
		return ErrInstalled
	}
	callbacks := db.Callback()
	// Verify the locked default pipeline before the first registration; no private members are patched.
	if callbacks.Create().Get("gorm:create") == nil || callbacks.Create().Get("gorm:save_after_associations") == nil || callbacks.Query().Get("gorm:query") == nil || callbacks.Query().Get("gorm:preload") == nil || callbacks.Update().Get("gorm:update") == nil || callbacks.Update().Get("gorm:save_after_associations") == nil || callbacks.Delete().Get("gorm:delete") == nil || callbacks.Delete().Get("gorm:after_delete") == nil || callbacks.Row().Get("gorm:row") == nil || callbacks.Raw().Get("gorm:raw") == nil {
		return ErrUnsupported
	}
	// Fixed registration entries keep rollback confined to this startup attempt.
	type callbackOwner interface {
		Get(string) func(*gorm.DB)
		Remove(string) error
	}
	registrations := []struct {
		owner    callbackOwner
		name     string
		register func() error
	}{
		{callbacks.Create(), pluginName + ":before_create", func() error {
			return callbacks.Create().Before("gorm:create").Register(pluginName+":before_create", p.before("create"))
		}},
		{callbacks.Create(), pluginName + ":after_create", func() error {
			return callbacks.Create().After("gorm:create").Before("gorm:save_after_associations").Register(pluginName+":after_create", p.after("create"))
		}},
		{callbacks.Query(), pluginName + ":before_query", func() error {
			return callbacks.Query().Before("gorm:query").Register(pluginName+":before_query", p.before("query"))
		}},
		{callbacks.Query(), pluginName + ":after_query", func() error {
			return callbacks.Query().After("gorm:query").Before("gorm:preload").Register(pluginName+":after_query", p.after("query"))
		}},
		{callbacks.Update(), pluginName + ":before_update", func() error {
			return callbacks.Update().Before("gorm:update").Register(pluginName+":before_update", p.before("update"))
		}},
		{callbacks.Update(), pluginName + ":after_update", func() error {
			return callbacks.Update().After("gorm:update").Before("gorm:save_after_associations").Register(pluginName+":after_update", p.after("update"))
		}},
		{callbacks.Delete(), pluginName + ":before_delete", func() error {
			return callbacks.Delete().Before("gorm:delete").Register(pluginName+":before_delete", p.before("delete"))
		}},
		{callbacks.Delete(), pluginName + ":after_delete", func() error {
			return callbacks.Delete().After("gorm:delete").Before("gorm:after_delete").Register(pluginName+":after_delete", p.after("delete"))
		}},
		{callbacks.Row(), pluginName + ":before_row", func() error {
			return callbacks.Row().Before("gorm:row").Register(pluginName+":before_row", p.before("row"))
		}},
		{callbacks.Row(), pluginName + ":after_row", func() error {
			return callbacks.Row().After("gorm:row").Register(pluginName+":after_row", p.after("row"))
		}},
		{callbacks.Raw(), pluginName + ":before_raw", func() error {
			return callbacks.Raw().Before("gorm:raw").Register(pluginName+":before_raw", p.before("raw"))
		}},
		{callbacks.Raw(), pluginName + ":after_raw", func() error {
			return callbacks.Raw().After("gorm:raw").Register(pluginName+":after_raw", p.after("raw"))
		}},
	}
	for _, registration := range registrations {
		if registration.owner.Get(registration.name) != nil {
			return ErrInstalled
		}
	}
	attempted := 0
	complete := false
	defer func() {
		if recover() != nil {
			failure = ErrConfiguration
		}
		if complete {
			return
		}
		// Register appends before compilation, including the failing entry.
		// Rollback is confined to this startup attempt, never runtime Close.
		var lastRemove error
		for index := attempted - 1; index >= 0; index-- {
			entry := registrations[index]
			lastRemove = entry.owner.Remove(entry.name)
		}
		// Intermediate compilations can still depend on another entry being removed.
		// The final compilation must restore the original chain; expose only a safe class.
		if lastRemove != nil {
			failure = errors.Join(ErrConfiguration, ErrRollback)
		}
	}()
	for index, registration := range registrations {
		attempted = index + 1
		if err := registration.register(); err != nil {
			return ErrConfiguration
		}
	}
	complete = true
	p.mu.Lock()
	p.ready = true
	p.mu.Unlock()
	return nil
}

type sampleStart struct{ when time.Time }

func (p *Plugin) before(operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		ctx := queryContext(tx)
		defer func() {
			if recover() != nil {
				p.notify(ctx, FailurePanic)
			}
		}()
		p.mu.Lock()
		enabled := p.ready && !p.status.Closed
		p.mu.Unlock()
		if !enabled || tx.DryRun || capture.IsInternal(ctx) {
			return
		}
		tx.Statement.Settings.Store(pluginName+":"+operation, sampleStart{p.config.Clock()})
	}
}
func (p *Plugin) after(operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		ctx := queryContext(tx)
		entered := false
		var failure Failure
		defer func() {
			if recover() != nil {
				failure = FailurePanic
			}
			if entered {
				p.leave()
			}
			if failure != "" {
				p.notify(ctx, failure)
			}
		}()
		value, exists := tx.Statement.Settings.LoadAndDelete(pluginName + ":" + operation)
		if !exists || tx.DryRun || capture.IsInternal(ctx) {
			return
		}
		start, ok := value.(sampleStart)
		if !ok {
			failure = FailureClock
			return
		}
		if tx.Statement.SQL.Len() == 0 {
			return
		}
		if tx.Error != nil && !errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			return
		}
		if row, ok := tx.Statement.Dest.(*sql.Row); ok && row.Err() != nil {
			return
		}
		if !p.enter() {
			return
		}
		entered = true
		failure = p.collect(ctx, tx, start.when)
	}
}
func queryContext(tx *gorm.DB) context.Context {
	if tx != nil && tx.Statement != nil && tx.Statement.Context != nil {
		return tx.Statement.Context
	}
	return context.Background()
}
func (p *Plugin) collect(ctx context.Context, tx *gorm.DB, begin time.Time) Failure {
	now := p.config.Clock()
	elapsed := now.Sub(begin)
	if elapsed < 0 {
		return FailureClock
	}
	input, err := PrepareSQL(tx.Dialector, tx.Statement.SQL.String(), tx.Statement.Vars)
	if err != nil {
		return FailureInput
	}
	captureCtx, cancel := context.WithTimeout(capture.InternalContext(ctx), p.config.CaptureTimeout)
	defer cancel()
	request := slowquery.Request{Method: "CLI"}
	if p.config.Request != nil {
		request, err = p.config.Request(captureCtx)
		if err != nil {
			return FailureRequest
		}
	}
	file, line := p.config.Callsite()
	connection := p.config.Connection
	input.TookMillis = float64(elapsed.Nanoseconds()) / 1e6
	input.File = file
	input.Line = line
	input.Connection = &connection
	input.Request = request
	_, accepted, err := p.config.Recorder.ReportSlowQuery(captureCtx, input, now)
	if err != nil {
		return FailureCapture
	}
	p.mu.Lock()
	if accepted {
		p.status.Captured++
	} else {
		p.status.Filtered++
	}
	p.mu.Unlock()
	return ""
}
func (p *Plugin) enter() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.status.Closed || !p.ready {
		return false
	}
	if p.status.InFlight == 0 {
		p.drained = make(chan struct{})
	}
	p.status.InFlight++
	return true
}
func (p *Plugin) leave() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status.InFlight--
	if p.status.InFlight == 0 {
		close(p.drained)
	}
}
func (p *Plugin) Close(ctx context.Context) error {
	if ctx == nil {
		return ErrConfiguration
	}
	deadline, cancel := context.WithTimeout(ctx, p.config.CaptureTimeout)
	defer cancel()
	p.mu.Lock()
	p.status.Closed = true
	if p.status.InFlight == 0 {
		p.mu.Unlock()
		return nil
	}
	drained := p.drained
	p.mu.Unlock()
	select {
	case <-drained:
		return nil
	case <-deadline.Done():
		return deadline.Err()
	}
}
func (p *Plugin) Status() Status { p.mu.Lock(); defer p.mu.Unlock(); return p.status }
func (p *Plugin) notify(ctx context.Context, kind Failure) {
	diagnostic := Diagnostic{Kind: kind}
	p.mu.Lock()
	p.status.Failures++
	sequence := p.status.Failures
	p.status.Last = diagnostic
	p.mu.Unlock()
	if p.config.OnError == nil {
		return
	}
	internal, cancel := context.WithTimeout(capture.InternalContext(ctx), p.config.CaptureTimeout)
	defer cancel()
	func() {
		defer func() {
			if recover() != nil {
				diagnostic.ObserverFailed = true
			}
		}()
		p.config.OnError(internal, diagnostic)
	}()
	if diagnostic.ObserverFailed {
		p.mu.Lock()
		p.status.ObserverPanics++
		if p.status.Failures == sequence {
			p.status.Last = diagnostic
		}
		p.mu.Unlock()
	}
}
func caller() (string, int) {
	var pcs [48]uintptr
	n := runtime.Callers(2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		ignored := false
		for _, prefix := range []string{"gitee.com/charsen/moo-go-monitor/", "github.com/goravel/framework/", "gorm.io/", "database/sql.", "runtime.", "testing."} {
			if strings.HasPrefix(frame.Function, prefix) {
				ignored = true
				break
			}
		}
		if !ignored {
			return frame.File, frame.Line
		}
		if !more {
			return "", 0
		}
	}
}
