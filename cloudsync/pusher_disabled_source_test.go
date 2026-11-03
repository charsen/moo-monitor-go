package cloudsync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitee.com/charsen/moo-go-monitor/buffer"
)

func TestPusherDisabledSourceInitializationIsolation(t *testing.T) {
	for _, disabled := range []string{"runtimes", "slow_sql"} {
		t.Run(disabled, func(t *testing.T) {
			ctx := context.Background()
			runtime := makeStore(t, privateRoot(t), "disabled-source")
			add(t, runtime, 301, clock)
			slow, input, now := slowSyncStore(t)
			if _, _, err := slow.ReportSlowQuery(ctx, input, now); err != nil {
				t.Fatal(err)
			}
			runtimeRoot, _, err := runtime.ScopeRoot()
			if err != nil {
				t.Fatal(err)
			}
			slowRoot, _, err := slow.ScopeRoot()
			if err != nil {
				t.Fatal(err)
			}
			badPath := filepath.Join(slowRoot, slowLockFile)
			if disabled == "runtimes" {
				badPath = filepath.Join(runtimeRoot, lockFile)
			}
			const marker = "owned disabled source lock"
			if err = os.WriteFile(badPath, []byte(marker), 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.Chmod(badPath, 0644); err != nil {
				t.Fatal(err)
			}
			runtimeCalls, slowCalls := 0, 0
			client := pushFixtureClient{
				runtime: func(_ context.Context, b []buffer.Snapshot) (Receipt, error) {
					runtimeCalls++
					if disabled == "runtimes" {
						t.Fatal("disabled runtime sent")
					}
					return Receipt{Legacy: true, Saved: int64(len(b))}, nil
				},
				slow: func(_ context.Context, b []buffer.SlowSnapshot) (Receipt, error) {
					slowCalls++
					if disabled == "slow_sql" {
						t.Fatal("disabled slow source sent")
					}
					return Receipt{Legacy: true, Saved: int64(len(b))}, nil
				},
			}
			// The malformed file really is a fatal constructor error when enabled.
			if invalid, e := NewPusher(runtime, slow, client, PushConfig{Runtimes: true, SlowSQL: true, Timeout: time.Second}); e == nil {
				invalid.Close(ctx)
				t.Fatal("enabled unsafe source accepted")
			}
			p, err := NewPusher(runtime, slow, client, PushConfig{Runtimes: disabled != "runtimes", SlowSQL: disabled != "slow_sql", Timeout: time.Second})
			if err != nil {
				t.Fatal("disabled source blocked enabled source", err)
			}
			defer p.Close(ctx)
			healthyTarget, badTarget := PushRuntimes, PushSlowSQL
			if disabled == "runtimes" {
				healthyTarget, badTarget = PushSlowSQL, PushRuntimes
			}
			stats, err := p.Stats(ctx, healthyTarget)
			if err != nil {
				t.Fatal("unselected initialization error leaked", err)
			}
			healthyStats, badStats := stats.Runtimes, stats.SlowSQL
			if disabled == "runtimes" {
				healthyStats, badStats = stats.SlowSQL, stats.Runtimes
			}
			if !healthyStats.Known || healthyStats.Counts == nil || healthyStats.Counts.PendingOpen != 1 || badStats.Skipped != "not_selected" {
				t.Fatal("healthy-only statistics lost", stats)
			}
			stats, err = p.Stats(ctx, PushBoth)
			healthyStats, badStats = stats.Runtimes, stats.SlowSQL
			if disabled == "runtimes" {
				healthyStats, badStats = stats.SlowSQL, stats.Runtimes
			}
			if !errors.Is(err, ErrPushSource) || !healthyStats.Known || healthyStats.Counts.PendingOpen != 1 || badStats.Known || badStats.Counts != nil || badStats.Reason != "initialization_failed" || badStats.Skipped != "" {
				t.Fatal("per-type statistics error lost", stats, err)
			}
			preview, err := p.Preview(ctx, healthyTarget, PreviewOptions{Mode: Incremental})
			if err != nil {
				t.Fatal("unselected preview error leaked", err)
			}
			goodPreview := preview.Runtimes
			if disabled == "runtimes" {
				goodPreview = preview.SlowSQL
			}
			if !goodPreview.Known || goodPreview.WouldSend == nil || *goodPreview.WouldSend != 1 {
				t.Fatal("healthy preview lost", preview)
			}
			preview, err = p.Preview(ctx, badTarget, PreviewOptions{Mode: Incremental})
			badPreview := preview.SlowSQL
			if disabled == "runtimes" {
				badPreview = preview.Runtimes
			}
			if !errors.Is(err, ErrPushSource) || badPreview.Known || badPreview.Selected != nil || badPreview.Counts != nil || badPreview.Reason != "initialization_failed" || badPreview.Skipped != "" {
				t.Fatal("bad preview became known empty", preview, err)
			}
			result, err := p.Push(ctx, PushBoth)
			if err != nil {
				t.Fatal("disabled init error leaked into transport", result, err)
			}
			good, bad := result.Runtimes, result.SlowSQL
			if disabled == "runtimes" {
				good, bad = result.SlowSQL, result.Runtimes
			}
			if good.Confirmed != 1 || bad.Skipped != "disabled" || bad.Failed {
				t.Fatal("disabled transport behavior changed", result)
			}
			result, err = p.Push(ctx, badTarget)
			bad = result.SlowSQL
			if disabled == "runtimes" {
				bad = result.Runtimes
			}
			if err != nil || bad.Skipped != "disabled" || runtimeCalls+slowCalls != 1 {
				t.Fatal("explicit disabled target behavior changed", result, err)
			}
			if err = p.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err = runtime.Snapshot(ctx); err != nil {
				t.Fatal("borrowed runtime closed", err)
			}
			if _, err = slow.Snapshot(ctx); err != nil {
				t.Fatal("borrowed slow buffer closed", err)
			}
			data, err := os.ReadFile(badPath)
			if err != nil || string(data) != marker {
				t.Fatal("disabled bad source modified", err)
			}
		})
	}
}

func TestPusherDisabledHealthySourcePreviewRemainsReadable(t *testing.T) {
	ctx := context.Background()
	slow, input, now := slowSyncStore(t)
	if _, _, err := slow.ReportSlowQuery(ctx, input, now); err != nil {
		t.Fatal(err)
	}
	p, err := NewPusher(nil, slow, pushFixtureClient{}, PushConfig{Runtimes: true, SlowSQL: false, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(ctx)
	result, err := p.Preview(ctx, PushSlowSQL, PreviewOptions{Mode: Incremental})
	if err != nil || !result.SlowSQL.Known || result.SlowSQL.WouldSend == nil || *result.SlowSQL.WouldSend != 1 || result.Runtimes.Skipped != "not_selected" {
		t.Fatal("disabled healthy source is no longer locally readable", result, err)
	}
}
