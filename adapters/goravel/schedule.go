package goravel

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"gitee.com/charsen/moo-monitor-go/capture"
	"gitee.com/charsen/moo-monitor-go/records"
	"github.com/goravel/framework/contracts/console"
)

var commandSignature = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_-]*$`)

type ScheduleConfig struct {
	Signature      func(string) (string, bool)
	Context        context.Context
	CaptureTimeout time.Duration
	OnError        OnError
}
type ScheduleReceipt struct {
	Signature string
	Err       error
	ExitCode  *int
	Runtime   *time.Duration
	Scope     *capture.EventScope
	EventID   string
}
type ScheduledArtisan struct {
	delegate    console.Artisan
	config      ScheduleConfig
	observation *lifecycleCapture
}

func NewScheduledArtisan(delegate console.Artisan, collector *capture.Collector, config ScheduleConfig) (*ScheduledArtisan, error) {
	if delegate == nil {
		return nil, capture.ErrCaptureInput
	}
	observer, err := newLifecycleCapture(collector, config.CaptureTimeout, config.OnError)
	if err != nil {
		return nil, err
	}
	if config.Context == nil {
		config.Context = context.Background()
	}
	return &ScheduledArtisan{delegate: delegate, config: config, observation: observer}, nil
}
func (a *ScheduledArtisan) Register(commands []console.Command) { a.delegate.Register(commands) }
func (a *ScheduledArtisan) CallAndExit(command string)          { a.delegate.CallAndExit(command) }
func (a *ScheduledArtisan) Run(args []string, exit bool) error  { return a.delegate.Run(args, exit) }
func (a *ScheduledArtisan) Status() Status                      { return a.observation.diagnostics.Status() }
func (a *ScheduledArtisan) Call(command string) error {
	started := time.Now()
	failure := a.delegate.Call(command)
	if failure != nil {
		a.observeCall(command, failure, time.Since(started))
	}
	return failure
}
func (a *ScheduledArtisan) observeCall(command string, failure error, duration time.Duration) {
	defer func() {
		if recover() != nil {
			a.observation.diagnose(a.config.Context, "schedule-call", ErrAdapter)
		}
	}()
	signature, trusted := a.resolveSignature(command)
	if !trusted {
		signature = ""
		a.observation.diagnose(a.config.Context, "schedule-signature", capture.ErrCaptureInput)
	}
	scope, _ := capture.NewEventScope(1)
	defer scope.Close()
	receipt := ScheduleReceipt{Signature: signature, Err: failure, Runtime: &duration, Scope: scope, EventID: "scheduled"}
	var coder interface{ ExitCode() int }
	if errors.As(failure, &coder) {
		code := coder.ExitCode()
		receipt.ExitCode = &code
	}
	_, _ = a.Complete(a.config.Context, receipt)
}

type ScheduledTaskExit struct {
	Signature string
	Code      int
}

func (e ScheduledTaskExit) Error() string {
	command := []rune(e.Signature)
	if len(command) > 300 {
		command = command[:300]
	}
	return fmt.Sprintf("调度任务退出码非零：%s (exit %d)", string(command), e.Code)
}

type ScheduledPanic struct{ Message string }

func (e ScheduledPanic) Error() string { return e.Message }

// Complete is an explicit owner receipt, not a claim that an arbitrary command was executed.
// Err is primary; an exit code accompanying it is metadata, never a second synthetic event.
func (a *ScheduledArtisan) Complete(ctx context.Context, receipt ScheduleReceipt) (hash string, failure error) {
	defer func() {
		if recover() != nil {
			hash = ""
			failure = ErrAdapter
			a.observation.diagnose(ctx, "schedule-complete", failure)
		}
	}()
	if ctx == nil {
		return "", capture.ErrCaptureInput
	}
	if capture.IsInternal(ctx) {
		return "", nil
	}
	if receipt.Err == nil && (receipt.ExitCode == nil || *receipt.ExitCode == 0) {
		return "", nil
	}
	trusted := commandSignature.MatchString(receipt.Signature)
	if !trusted && receipt.Err == nil {
		return "", capture.ErrCaptureInput
	}
	scope, id := receipt.Scope, receipt.EventID
	if scope == nil && id == "" {
		scope, id = capture.EventFromContext(ctx)
	}
	if scope == nil || id == "" {
		return "", capture.ErrEventIdentity
	}
	meta := map[string]any{"exit_code": nil}
	if trusted {
		meta["command"] = receipt.Signature
	}
	if receipt.ExitCode != nil {
		meta["exit_code"] = int64(*receipt.ExitCode)
	}
	if receipt.Runtime != nil {
		if *receipt.Runtime < 0 {
			return "", capture.ErrCaptureInput
		}
		// Positive duration rounds to centiseconds without multiplying an imprecise float.
		units := *receipt.Runtime / (10 * time.Millisecond)
		if *receipt.Runtime%(10*time.Millisecond) >= 5*time.Millisecond {
			units++
		}
		meta["runtime"] = float64(units) / 100
	}
	options := capture.Options{Scope: scope, EventID: id, Source: "reportable", Meta: meta}
	event := receipt.Err
	if event == nil {
		// Only an explicitly known command signature controls feedback suppression.
		if len(receipt.Signature) > len("moo:cloud:") && strings.HasPrefix(strings.ToLower(receipt.Signature), "moo:cloud:") {
			return "", nil
		}
		event = ScheduledTaskExit{Signature: receipt.Signature, Code: *receipt.ExitCode}
		options.Source = "schedule_exit"
		options.Frames = []records.Frame{{File: "moo-monitor/schedule", Function: "ScheduledTaskExit"}}
	}
	return a.observation.record(ctx, event, options, "schedule-complete")
}

// Callback preserves the original panic for the real scheduler's cron.Recover chain.
func (a *ScheduledArtisan) Callback(signature string, callback func()) (func(), error) {
	if callback == nil || !commandSignature.MatchString(signature) {
		return nil, capture.ErrCaptureInput
	}
	return func() {
		started := time.Now()
		defer func() {
			if value := recover(); value != nil {
				func() {
					defer func() {
						if recover() != nil {
							a.observation.diagnose(a.config.Context, "schedule-panic", ErrAdapter)
						}
					}()
					scope, _ := capture.NewEventScope(1)
					defer scope.Close()
					event, ok := value.(error)
					if !ok {
						message := "schedule panic"
						if text, yes := value.(string); yes {
							message = text
						} else {
							message += " (" + reflect.TypeOf(value).String() + ")"
						}
						event = ScheduledPanic{Message: message}
					}
					duration := time.Since(started)
					_, _ = a.Complete(a.config.Context, ScheduleReceipt{Signature: signature, Err: event, Runtime: &duration, Scope: scope, EventID: "scheduled"})
				}()
				panic(value)
			}
		}()
		callback()
	}, nil
}

func (a *ScheduledArtisan) resolveSignature(command string) (signature string, trusted bool) {
	defer func() {
		if recover() != nil {
			signature = ""
			trusted = false
		}
	}()
	if a.config.Signature == nil {
		return "", false
	}
	signature, trusted = a.config.Signature(command)
	return signature, trusted && commandSignature.MatchString(signature)
}
