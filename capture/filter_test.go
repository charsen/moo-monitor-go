package capture

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"gitee.com/charsen/moo-go-monitor/records"
)

type statusError int

func (e statusError) Error() string       { return "fixed status event" }
func (e statusError) HTTPStatusCode() int { return int(e) }

type inputError struct{}

func (inputError) Error() string           { return "fixed input usage" }
func (inputError) ConsoleInputError() bool { return true }

type panicFilter struct{}

func (panicFilter) Error() string       { return "not read" }
func (panicFilter) HTTPStatusCode() int { panic("private filter cause") }
func TestExplicitFilters(t *testing.T) {
	for _, tc := range []struct {
		name, method     string
		event            error
		experiment, skip bool
	}{{"http404", "GET", statusError(404), false, true}, {"http500", "POST", statusError(500), false, false}, {"wrapped404", "POST", fmt.Errorf("outer: %w", statusError(404)), false, true}, {"consoleCLI", "CLI", inputError{}, false, true}, {"consoleHTTP", "POST", inputError{}, false, false}, {"experimentCLI", "", errors.New("experiment"), true, true}, {"experimentHTTP", "POST", errors.New("experiment"), true, false}, {"join", "CLI", errors.Join(statusError(404), errors.New("real business failure")), false, false}, {"invalid-status-low", "GET", statusError(99), false, false}, {"invalid-status-high", "GET", statusError(600), false, false}, {"unknown", "CLI", errors.New("404 not a capability"), false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			collector, store := setup(t)
			scope, _ := NewEventScope(1)
			defer scope.Close()
			options := Options{Scope: scope, EventID: "same", Request: records.Request{Method: tc.method}, CLIExperiment: tc.experiment, Frames: frames}
			hash, err := collector.Capture(context.Background(), tc.event, options)
			if err != nil || (hash == "") != tc.skip {
				t.Fatal(hash, err)
			}
			snapshots, err := store.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if tc.skip {
				if len(snapshots) != 0 || len(scope.entries) != 0 {
					t.Fatal("filtered event occupied scope")
				}
				options.CLIExperiment = false
				if _, err := collector.Capture(context.Background(), errors.New("real event"), options); err != nil {
					t.Fatal("ID not reusable", err)
				}
			} else if len(snapshots) != 1 {
				t.Fatal("unknown event lost")
			}
		})
	}
	collector, store := setup(t)
	scope, _ := NewEventScope(1)
	defer scope.Close()
	options := Options{Scope: scope, EventID: "same", Frames: frames}
	hash, err := collector.Capture(context.Background(), panicFilter{}, options)
	if hash != "" || !errors.Is(err, ErrExtraction) || len(scope.entries) != 0 {
		t.Fatal("filter panic state", hash, err)
	}
	disabled, err := New(store, Config{Filter: &FilterConfig{}})
	if err != nil {
		t.Fatal(err)
	}
	if hash, err := disabled.Capture(context.Background(), inputError{}, options); err != nil || hash == "" {
		t.Fatal("explicit console override", err)
	}
}
