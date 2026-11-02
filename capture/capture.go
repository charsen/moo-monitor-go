// Package capture turns explicit errors/messages into existing sanitized buffer records.
package capture

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	"gitee.com/charsen/moo-go-monitor/buffer"
	"gitee.com/charsen/moo-go-monitor/records"
	"gitee.com/charsen/moo-go-monitor/sourcecontext"
)

var ErrExtraction = errors.New("capture error extraction panicked")

// ErrSourceContext reports auxiliary display enrichment failure; a nonempty hash may already be saved.
var ErrSourceContext = errors.New("capture source context unavailable")
var ErrCaptureInput = errors.New("invalid capture input")

type Config struct {
	Env, Project, GoVersion, GoravelVersion string
	Clock                                   func() time.Time
	Filter                                  *FilterConfig
	SourceReader                            SourceReader
}

// SourceReader is borrowed; its owner closes it after capture users stop.
type SourceReader interface {
	Read(context.Context, []records.Frame) (sourcecontext.Details, error)
}

type Options struct {
	Scope           *EventScope
	EventID, Source string
	Request         records.Request
	Payload         any
	Meta            map[string]any
	Frames          []records.Frame
	CLIExperiment   bool
}
type Recorder interface {
	Record(context.Context, records.Snapshot, time.Time) (string, error)
	TagSource(context.Context, string, string, map[string]any, time.Time) (bool, error)
}
type Collector struct {
	store  Recorder
	config Config
	filter FilterConfig
}

func New(store Recorder, config Config) (*Collector, error) {
	if store == nil {
		return nil, ErrCaptureInput
	}
	if config.GoVersion == "" {
		config.GoVersion = runtime.Version()
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	filter := FilterConfig{SkipConsoleInput: true, SkipCLIExperiment: true}
	if config.Filter != nil {
		filter = *config.Filter
	}
	config.Filter = nil
	return &Collector{store: store, config: config, filter: filter}, nil
}
func (c *Collector) Capture(ctx context.Context, err error, options Options) (hash string, failure error) {
	if ctx == nil || err == nil {
		return "", ErrCaptureInput
	}
	return c.capture(ctx, err, "", options)
}
func (c *Collector) LogMessage(ctx context.Context, message string, options Options) (string, error) {
	if ctx == nil {
		return "", ErrCaptureInput
	}
	options.Source = "log_message"
	return c.capture(ctx, nil, message, options)
}

func (c *Collector) capture(ctx context.Context, event error, message string, options Options) (hash string, failure error) {
	recording := false
	defer func() {
		if recover() != nil {
			hash = ""
			failure = ErrExtraction
			if recording {
				failure = errors.Join(failure, ErrEventUncertain)
			}
		}
	}()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if c.shouldSkip(event, options) {
		return "", nil
	}
	if options.Scope == nil && options.EventID == "" {
		options.Scope, options.EventID = EventFromContext(ctx)
	}
	scoped := options.Scope != nil || options.EventID != ""
	if scoped && (options.Scope == nil || !validID(options.EventID)) {
		return "", ErrEventIdentity
	}
	source := records.NormalizeSource(options.Source)
	if options.Source == "" {
		source = "reportable"
	}
	if options.Scope != nil {
		scope := options.Scope
		if err := scope.enter(ctx); err != nil {
			return "", err
		}
		defer scope.leave()
		if previous, exists := scope.entries[options.EventID]; exists {
			if previous.uncertain {
				return previous.hash, ErrEventUncertain
			}
			if records.SourceRank(source) > previous.rank {
				now := c.config.Clock()
				pending := previous
				pending.uncertain = true
				scope.entries[options.EventID] = pending
				recording = true
				tagged, err := c.store.TagSource(ctx, previous.hash, source, options.Meta, now)
				recording = false
				if err != nil {
					if errors.Is(err, buffer.ErrOutcomeUnknown) {
						previous.uncertain = true
					}
					scope.entries[options.EventID] = previous
					return previous.hash, err
				}
				if !tagged {
					scope.entries[options.EventID] = previous
					return previous.hash, errors.New("capture event record is no longer present")
				}
				previous.rank = records.SourceRank(source)
				scope.entries[options.EventID] = previous
			}
			return previous.hash, nil
		}
		if len(scope.entries) >= scope.max {
			return "", ErrScopeFull
		}
	}
	snapshot, err := c.snapshot(event, message, source, options)
	if err != nil {
		return "", err
	}
	sourceErr := c.enrichSource(ctx, &snapshot)
	now := c.config.Clock()
	if options.Scope != nil {
		options.Scope.entries[options.EventID] = scopeEntry{uncertain: true}
	}
	recording = true
	hash, err = c.store.Record(ctx, snapshot, now)
	recording = false
	if options.Scope != nil {
		if err == nil {
			options.Scope.entries[options.EventID] = scopeEntry{hash: hash, rank: records.SourceRank(source)}
		} else if errors.Is(err, buffer.ErrOutcomeUnknown) {
			options.Scope.entries[options.EventID] = scopeEntry{hash: hash, uncertain: true}
		} else {
			delete(options.Scope.entries, options.EventID)
		}
	}
	if sourceErr != nil {
		return hash, errors.Join(err, sourceErr)
	}
	return hash, err
}

// Enrichment happens only after exception identity and original trace are fixed.
func (c *Collector) enrichSource(ctx context.Context, snapshot *records.Snapshot) (failure error) {
	if c.config.SourceReader == nil {
		return nil
	}
	frames := append([]records.Frame(nil), snapshot.Trace.Frames...)
	defer func() {
		if recover() != nil {
			failure = ErrSourceContext
		}
	}()
	details, err := c.config.SourceReader.Read(ctx, frames)
	if err != nil {
		return ErrSourceContext
	}
	snapshot.Trace.Frames = details.Frames
	snapshot.Snippet = details.Snippet
	return nil
}

func (c *Collector) snapshot(event error, message, source string, options Options) (records.Snapshot, error) {
	frames := options.Frames
	if frames == nil {
		frames = CallerFrames(2, nil)
	}
	frames = append([]records.Frame{}, frames[:min(len(frames), 20)]...)
	exception := records.Exception{Class: "LoggedErrorMessage", Message: message}
	if len(frames) > 0 {
		exception.File = frames[0].File
		exception.Line = frames[0].Line
	}
	if event != nil {
		exception.Class = reflect.TypeOf(event).String()
		exception.Message = event.Error()
		queue := unwrap(event)
		for len(queue) > 0 && len(exception.Previous) < 3 {
			current := queue[0]
			queue = queue[1:]
			if current == nil {
				continue
			}
			exception.Previous = append(exception.Previous, records.Previous{Class: reflect.TypeOf(current).String(), Message: current.Error()})
			if len(exception.Previous) < 3 {
				children := unwrap(current)
				queue = append(queue, children[:min(len(children), 3-len(exception.Previous))]...)
			}
		}
	}
	var full strings.Builder
	for _, frame := range frames {
		fmt.Fprintf(&full, "%s:%d %s\n", frame.File, frame.Line, frame.Function)
	}
	request := options.Request
	if request.Method == "" {
		request.Method = "CLI"
	}
	return records.Snapshot{Exception: exception, Request: request, Context: records.GoContext{Env: c.config.Env, Project: c.config.Project, Go: c.config.GoVersion, Goravel: c.config.GoravelVersion}, Trace: records.Trace{Full: full.String(), Frames: frames}, Payload: options.Payload, Source: source, Meta: options.Meta}, nil
}
func unwrap(err error) []error {
	if single, ok := err.(interface{ Unwrap() error }); ok {
		if child := single.Unwrap(); child != nil {
			return []error{child}
		}
		return nil
	}
	if multiple, ok := err.(interface{ Unwrap() []error }); ok {
		children := multiple.Unwrap()
		return children[:min(len(children), 3)]
	}
	return nil
}

// CallerFrames reads runtime caller positions only, never arbitrary source files or error properties.
func CallerFrames(skip int, exclude []string) []records.Frame {
	var pcs [64]uintptr
	n := runtime.Callers(skip+1, pcs[:])
	iterator := runtime.CallersFrames(pcs[:n])
	result := []records.Frame{}
	for {
		frame, more := iterator.Next()
		ignored := false
		for _, prefix := range append([]string{"gitee.com/charsen/moo-go-monitor/capture.", "gitee.com/charsen/moo-go-monitor/adapters/goravel."}, exclude...) {
			if strings.HasPrefix(frame.Function, prefix) {
				ignored = true
				break
			}
		}
		if !ignored {
			result = append(result, records.Frame{File: frame.File, Line: frame.Line, Function: frame.Function})
			if len(result) == 20 {
				break
			}
		}
		if !more {
			break
		}
	}
	return result
}
