package goravel_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	adapter "gitee.com/charsen/moo-monitor-go/adapters/goravel"
	"gitee.com/charsen/moo-monitor-go/capture"
	frameworkconsole "github.com/goravel/framework/console"
	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"
	"github.com/goravel/framework/contracts/log"
	"github.com/goravel/framework/contracts/schedule"
	frameworkschedule "github.com/goravel/framework/schedule"
)

type artisanFixture struct {
	failure    error
	registered int
	runCalls   int
	exitCalls  int
}

func (a *artisanFixture) Register(commands []console.Command) { a.registered += len(commands) }
func (a *artisanFixture) Call(string) error                   { return a.failure }
func (a *artisanFixture) CallAndExit(string)                  { a.exitCalls++ }
func (a *artisanFixture) Run([]string, bool) error            { a.runCalls++; return a.failure }

type returnedCode struct{ code int }

func (e *returnedCode) Error() string { return "returned typed exit" }
func (e *returnedCode) ExitCode() int { return e.code }
func TestScheduledArtisanPreservesReturns(t *testing.T) {
	collector, store := setup(t)
	native := &artisanFixture{}
	observed, err := adapter.NewScheduledArtisan(native, collector, adapter.ScheduleConfig{Signature: func(string) (string, bool) { return "fixture:run", true }})
	if err != nil {
		t.Fatal(err)
	}
	if err = observed.Call("fixture:run token=fake-argument"); err != nil || totalRuntime(t, store) != 0 {
		t.Fatal("success captured")
	}
	failure := errors.New("actual command failure")
	native.failure = failure
	if err = observed.Call("fixture:run token=fake-argument"); err != failure {
		t.Fatal("original Call error replaced")
	}
	all, _ := store.Snapshot(context.Background())
	if len(all) != 1 || all[0].Record.Meta["command"] != "fixture:run" || all[0].Record.Meta["exit_code"] != nil {
		t.Fatal("unknown exit inferred or signature wrong")
	}
	if strings.Contains(fmt.Sprint(all), "fake-argument") {
		t.Fatal("full command arguments retained")
	}
	code := &returnedCode{code: 7}
	native.failure = code
	if err = observed.Call("fixture:run"); err != code {
		t.Fatal("typed error replaced")
	}
	observed.Register([]console.Command{&commandFixture{name: "fixture"}})
	observed.CallAndExit("fixture")
	if observed.Run(nil, false) != code || native.runCalls != 1 || native.exitCalls != 1 || native.registered != 1 {
		t.Fatal("non-Call public actions changed")
	}
}
func TestScheduleExplicitCompletion(t *testing.T) {
	collector, store := setup(t)
	observed, err := adapter.NewScheduledArtisan(&artisanFixture{}, collector, adapter.ScheduleConfig{})
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := capture.NewEventScope(8)
	defer scope.Close()
	zero, two := 0, 2
	duration := 1005 * time.Millisecond
	receipt := adapter.ScheduleReceipt{Signature: "backup:run", ExitCode: &zero, Scope: scope, EventID: "exit", Runtime: &duration}
	if _, err = observed.Complete(context.Background(), receipt); err != nil || totalRuntime(t, store) != 0 {
		t.Fatal("successful completion captured")
	}
	receipt.ExitCode = &two
	if _, err = observed.Complete(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	if _, err = observed.Complete(context.Background(), receipt); err != nil || totalRuntime(t, store) != 1 {
		t.Fatal("explicit event ID counted twice", err)
	}
	all, _ := store.Snapshot(context.Background())
	record := all[0].Record
	if record.Meta["source"] != "schedule_exit" || record.Exception.File != "moo-monitor/schedule" || fmt.Sprint(record.Meta["runtime"]) != "1.01" {
		t.Fatal("scheduled exit contract missing")
	}
	cloud := receipt
	cloud.Signature = "moo:cloud:push"
	cloud.EventID = "cloud"
	if _, err = observed.Complete(context.Background(), cloud); err != nil || totalRuntime(t, store) != 1 {
		t.Fatal("synthetic cloud feedback captured")
	}
	cloud.Err = errors.New("real cloud code defect")
	if _, err = observed.Complete(context.Background(), cloud); err != nil || totalRuntime(t, store) != 2 {
		t.Fatal("real cloud error was swallowed")
	}
	if _, err = observed.Complete(context.Background(), adapter.ScheduleReceipt{Signature: "backup:run", ExitCode: &two}); !errors.Is(err, capture.ErrEventIdentity) {
		t.Fatal("missing explicit identity accepted")
	}
}
func TestScheduleUntrustedSignatureAndPanic(t *testing.T) {
	collector, store := setup(t)
	failure := errors.New("trusted actual failure")
	observed, err := adapter.NewScheduledArtisan(&artisanFixture{failure: failure}, collector, adapter.ScheduleConfig{Signature: func(string) (string, bool) { panic("fake-token-resolver") }})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Call("untrusted --token=fake-argument") != failure || observed.Status().Failures == 0 || totalRuntime(t, store) != 1 {
		t.Fatal("signature resolver changed original failure or hid it")
	}
	all, _ := store.Snapshot(context.Background())
	if _, exists := all[0].Record.Meta["command"]; exists {
		t.Fatal("untrusted command retained")
	}
	var called bool
	value := &panicStringer{called: &called}
	callback, err := observed.Callback("fixture:callback", func() { panic(value) })
	if err != nil {
		t.Fatal(err)
	}
	var recovered any
	func() { defer func() { recovered = recover() }(); callback() }()
	if recovered != value || called {
		t.Fatal("panic value was replaced or Stringer invoked")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	broken, err := adapter.NewScheduledArtisan(&artisanFixture{}, collector, adapter.ScheduleConfig{OnError: func(context.Context, adapter.Diagnostic) { panic("observer fixture") }})
	if err != nil {
		t.Fatal(err)
	}
	callback, err = broken.Callback("fixture:callback", func() { panic(failure) })
	if err != nil {
		t.Fatal(err)
	}
	recovered = nil
	func() { defer func() { recovered = recover() }(); callback() }()
	if recovered != failure || !broken.Status().Last.ObserverFailed {
		t.Fatal("monitor/observer failure replaced business panic")
	}
}

type panicStringer struct{ called *bool }

func (v *panicStringer) String() string { *v.called = true; return "fake-token-object" }

type commandFixture struct {
	name    string
	failure error
}

func (c *commandFixture) Signature() string            { return c.name }
func (*commandFixture) Description() string            { return "fixed local fixture" }
func (*commandFixture) Extend() command.Extend         { return command.Extend{} }
func (c *commandFixture) Handle(console.Context) error { return c.failure }

type scheduleLog struct {
	log.Log
	once sync.Once
	done chan struct{}
}

func (l *scheduleLog) Error(...any) { l.once.Do(func() { close(l.done) }) }
func TestRealScheduleCronAndArtisan(t *testing.T) {
	collector, store := setup(t)
	native := frameworkconsole.NewApplication("fixture", "", "", "", true)
	native.Register([]console.Command{&commandFixture{name: "fixture:command", failure: errors.New("command failure")}})
	observed, err := adapter.NewScheduledArtisan(native, collector, adapter.ScheduleConfig{Signature: func(string) (string, bool) { return "fixture:command", true }})
	if err != nil {
		t.Fatal(err)
	}
	original := errors.New("callback panic")
	callback, err := observed.Callback("fixture:callback", func() { panic(original) })
	if err != nil {
		t.Fatal(err)
	}
	logger := &scheduleLog{done: make(chan struct{})}
	scheduler := frameworkschedule.NewApplication(observed, nil, logger, false)
	scheduler.Register([]schedule.Event{scheduler.Call(callback).EverySecond(), scheduler.Command("fixture:command").EverySecond()})
	done := make(chan struct{})
	go func() { scheduler.Run(); close(done) }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer scheduler.Shutdown(ctx)
	select {
	case <-logger.done:
	case <-ctx.Done():
		t.Fatal("original cron recovery/log did not run")
	}
	// Wait for both callbacks in this same tick, then stop the original scheduler.
	until := time.Now().Add(time.Second)
	for totalRuntime(t, store) < 2 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if err := scheduler.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("scheduler did not stop")
	}
	if totalRuntime(t, store) != 2 {
		t.Fatal("real command/callback failures not captured once")
	}
}
