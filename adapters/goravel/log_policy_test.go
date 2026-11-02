package goravel_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	adapter "gitee.com/charsen/moo-go-monitor/adapters/goravel"
	"gitee.com/charsen/moo-go-monitor/capture"
	"github.com/goravel/framework/contracts/log"
	frameworklog "github.com/goravel/framework/log"
)

func TestLogPolicyWriterSourcesAndLevels(t *testing.T) {
	for _, test := range []struct {
		name                       string
		policy                     *adapter.LogPolicy
		contextCount, messageCount int
	}{
		{"context-only", &adapter.LogPolicy{ContextEnabled: true, Levels: []log.Level{log.LevelWarning}}, 1, 0},
		{"message-only", &adapter.LogPolicy{MessageEnabled: true, Levels: []log.Level{log.LevelWarning}}, 0, 1},
		{"empty-levels", &adapter.LogPolicy{ContextEnabled: true, MessageEnabled: true}, 0, 0},
		{"both-off", &adapter.LogPolicy{Levels: []log.Level{log.LevelWarning}}, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			collector, store := setup(t)
			native := &delegate{}
			driver, err := adapter.NewDriverWithPolicy(collector, native, nil, test.policy)
			if err != nil {
				t.Fatal(err)
			}
			logger := writer(t, context.Background(), driver)
			logger.With(map[string]any{adapter.ExceptionKey: errors.New("fixed context"), adapter.FramesKey: fixedFrames}).Warning("context message")
			logger.With(map[string]any{adapter.FramesKey: fixedFrames}).Warning("fixed standalone message")
			logger.With(map[string]any{adapter.FramesKey: fixedFrames}).Error("not in explicit set")
			snapshots, err := store.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			contexts, messages := 0, 0
			for _, s := range snapshots {
				switch s.Record.Meta["source"] {
				case "log_context":
					contexts++
				case "log_message":
					messages++
				}
			}
			if contexts != test.contextCount || messages != test.messageCount || len(native.messages) != 3 {
				t.Fatal("policy/delegate", contexts, messages, len(native.messages))
			}
		})
	}
}
func TestLogPolicyCopyValidationAndScope(t *testing.T) {
	collector, store := setup(t)
	if _, err := adapter.NewDriverWithPolicy(collector, nil, nil, &adapter.LogPolicy{Levels: []log.Level{log.Level(9)}}); !errors.Is(err, adapter.ErrLogPolicy) {
		t.Fatal("invalid level", err)
	}
	input := &adapter.LogPolicy{MessageEnabled: true, Levels: []log.Level{log.LevelWarning}}
	native := &delegate{}
	driver, err := adapter.NewDriverWithPolicy(collector, native, nil, input)
	if err != nil {
		t.Fatal(err)
	}
	input.MessageEnabled = false
	input.Levels[0] = log.LevelError
	logger := writer(t, context.Background(), driver)
	logger.With(map[string]any{adapter.FramesKey: fixedFrames}).Warning("immutable policy")
	snapshots, _ := store.Snapshot(context.Background())
	if len(snapshots) != 1 {
		t.Fatal("caller changed policy")
	}
	scope, _ := capture.NewEventScope(1)
	defer scope.Close()
	ctx := capture.WithEvent(context.Background(), scope, "same")
	event := errors.New("source filtered before identity")
	writer(t, ctx, driver).With(map[string]any{adapter.ExceptionKey: event, adapter.FramesKey: fixedFrames}).Warning("must not downgrade")
	hash, err := collector.Capture(ctx, event, capture.Options{Scope: scope, EventID: "same", Source: "reportable", Frames: fixedFrames})
	if err != nil || hash == "" {
		t.Fatal("filtered context occupied identity", err)
	}
	snapshots, _ = store.Snapshot(context.Background())
	if len(snapshots) != 2 {
		t.Fatal("context downgraded")
	}
	legacy, _ := adapter.NewDriver(collector, nil, nil)
	if legacy.Handler.Enabled(log.LevelInfo) || !legacy.Handler.Enabled(log.Level(9)) {
		t.Fatal("nil legacy changed")
	}
	failure := errors.New("original delegate failure")
	native.failure = failure
	entry := frameworklog.FromSlogRecord(slog.NewRecord(clock, slog.LevelInfo, "ordinary info", 0))
	if err = driver.Handler.Handle(entry); !errors.Is(err, failure) {
		t.Fatal("filtered delegate error changed", err)
	}
}
