package cloudsync

import (
	"context"
	"errors"
)

// SelectionMode names the candidate policy, independently of transport.
type SelectionMode string

const Incremental SelectionMode = "incremental"

type PreviewOptions struct{ Mode SelectionMode }

type TypePreview struct {
	TypePendingStats
	// Nil means unknown, never a known empty candidate set.
	Selected, WouldSend                   *int
	ExcludedConfirmed, ExcludedSuppressed *int
}
type PushPreview struct{ Runtimes, SlowSQL TypePreview }

// Preview inspects one locked strict snapshot. Mode controls only the local
// candidate policy; it does not verify Cloud acceptance or configuration.
func (r *StatusReader) Preview(ctx context.Context, target PushTarget, options PreviewOptions) (PushPreview, error) {
	result := PushPreview{Runtimes: TypePreview{TypePendingStats: TypePendingStats{Skipped: "not_selected"}}, SlowSQL: TypePreview{TypePendingStats: TypePendingStats{Skipped: "not_selected"}}}
	if ctx == nil || !validSelectionMode(options.Mode) || (target != PushBoth && target != PushRuntimes && target != PushSlowSQL) {
		return result, ErrPushConfig
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var failure error
	if target == PushBoth || target == PushRuntimes {
		if r.runtime == nil {
			result.Runtimes.Skipped = "source_unavailable"
			result.Runtimes.Reason = "unavailable"
		} else {
			value, err := r.runtime.inspect(ctx, options.Mode)
			result.Runtimes = value
			failure = errors.Join(failure, err)
		}
	}
	if target == PushBoth || target == PushSlowSQL {
		if r.slow == nil {
			result.SlowSQL.Skipped = "source_unavailable"
			result.SlowSQL.Reason = "unavailable"
		} else {
			value, err := r.slow.inspect(ctx, options.Mode)
			result.SlowSQL = value
			failure = errors.Join(failure, err)
		}
	}
	return result, failure
}
func (s *synchronizer[R]) Preview(ctx context.Context, options PreviewOptions) (TypePreview, error) {
	return s.inspect(ctx, options.Mode)
}
func (p *Pusher) Preview(ctx context.Context, target PushTarget, options PreviewOptions) (PushPreview, error) {
	result := PushPreview{Runtimes: TypePreview{TypePendingStats: TypePendingStats{Skipped: "not_selected"}}, SlowSQL: TypePreview{TypePendingStats: TypePendingStats{Skipped: "not_selected"}}}
	if !validSelectionMode(options.Mode) || (target != PushBoth && target != PushRuntimes && target != PushSlowSQL) {
		return result, ErrPushConfig
	}
	operation, leave, err := p.enterOperation(ctx)
	if err != nil {
		return result, err
	}
	defer leave()
	var failure error
	if target == PushBoth || target == PushRuntimes {
		if p.runtimeInitError != nil {
			result.Runtimes.Skipped = ""
			result.Runtimes.Reason = "initialization_failed"
			failure = errors.Join(failure, ErrPushSource, p.runtimeInitError)
		} else if p.runtime == nil {
			result.Runtimes.Skipped = "source_unavailable"
			failure = errors.Join(failure, ErrPushSource)
		} else {
			value, err := p.runtime.inspect(operation, options.Mode)
			result.Runtimes = value
			failure = errors.Join(failure, err)
		}
	}
	if target == PushBoth || target == PushSlowSQL {
		if p.slowInitError != nil {
			result.SlowSQL.Skipped = ""
			result.SlowSQL.Reason = "initialization_failed"
			failure = errors.Join(failure, ErrPushSource, p.slowInitError)
		} else if p.slow == nil {
			result.SlowSQL.Skipped = "source_unavailable"
			failure = errors.Join(failure, ErrPushSource)
		} else {
			value, err := p.slow.inspect(operation, options.Mode)
			result.SlowSQL = value
			failure = errors.Join(failure, err)
		}
	}
	return result, failure
}
