package cloudsync

import (
	"context"
	"errors"

	"gitee.com/charsen/moo-monitor-go/buffer"
)

type PruneOptions struct {
	RetentionDays int
	DryRun        bool
}
type PruneResult struct {
	Known, Busy, Disabled                          bool
	Candidates, Purged, ChangedOrPreserved, Failed int
	OutcomeUnknown                                 bool
	Reason                                         string
}
type TypePruneResult struct {
	PruneResult
	Skipped string
}
type PushPruneResult struct{ Runtimes, SlowSQL TypePruneResult }

type resolvedPruner[R any] interface {
	readOnlySnapshotStore[R]
	PruneResolved(context.Context, buffer.VersionedSnapshot[R]) (bool, error)
}

// Prune is an explicit local maintenance action, not a network or age policy.
// RetentionDays <= 0 disables the operation without reading or writing stores.
func (s *synchronizer[R]) Prune(ctx context.Context, options PruneOptions) (PruneResult, error) {
	result := PruneResult{}
	if ctx == nil {
		return result, ErrPushConfig
	}
	if options.RetentionDays <= 0 {
		result.Known = true
		result.Disabled = true
		return result, nil
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	select {
	case <-s.gate:
		defer func() { s.gate <- struct{}{} }()
	default:
		result.Busy = true
		result.Reason = "busy"
		return result, nil
	}
	if s.root == nil {
		result.Reason = "closed"
		return result, ErrPushClosed
	}
	lock, ok, err := s.lock()
	if err != nil {
		result.Reason = "lock"
		return result, err
	}
	if !ok {
		result.Busy = true
		result.Reason = "busy"
		return result, nil
	}
	defer lock.Close()
	ledger, err := s.load()
	if err != nil {
		result.Reason = "state"
		return result, err
	}
	if _, err = s.loadSuppression(); err != nil {
		result.Reason = "suppression"
		return result, err
	}
	source, ok := s.store.(resolvedPruner[R])
	if !ok {
		result.Reason = "unsupported_store"
		return result, ErrPushSource
	}
	snapshots, err := source.ReadOnlySnapshot(ctx)
	if err != nil {
		result.Reason = "snapshot"
		if errors.Is(err, buffer.ErrRecoveryRequired) {
			result.Reason = "recovery_required"
		}
		return result, err
	}
	result.Known = true
	var failure error
	for _, snapshot := range snapshots {
		if snapshot.Bucket != "resolved" || !matches(ledger.Confirmed[snapshot.Hash], snapshot) {
			continue
		}
		result.Candidates++
		if options.DryRun {
			continue
		}
		applied, err := source.PruneResolved(ctx, snapshot)
		if err != nil {
			result.Failed++
			result.OutcomeUnknown = true
			failure = errors.Join(failure, err)
			continue
		}
		if applied {
			result.Purged++
		} else {
			result.ChangedOrPreserved++
		}
	}
	return result, failure
}

// Prune never needs a Cloud client. Existing reader construction may prepare locks.
func (r *StatusReader) Prune(ctx context.Context, target PushTarget, options PruneOptions) (PushPruneResult, error) {
	result := PushPruneResult{Runtimes: TypePruneResult{Skipped: "not_selected"}, SlowSQL: TypePruneResult{Skipped: "not_selected"}}
	if ctx == nil || (target != PushBoth && target != PushRuntimes && target != PushSlowSQL) {
		return result, ErrPushConfig
	}
	var failure error
	if target == PushBoth || target == PushRuntimes {
		if options.RetentionDays <= 0 {
			result.Runtimes = TypePruneResult{PruneResult: PruneResult{Known: true, Disabled: true}}
		} else if r.runtime == nil {
			result.Runtimes.Skipped = "source_unavailable"
			result.Runtimes.Reason = "unavailable"
		} else {
			value, err := r.runtime.Prune(ctx, options)
			result.Runtimes = TypePruneResult{PruneResult: value}
			failure = errors.Join(failure, err)
		}
	}
	if target == PushBoth || target == PushSlowSQL {
		if options.RetentionDays <= 0 {
			result.SlowSQL = TypePruneResult{PruneResult: PruneResult{Known: true, Disabled: true}}
		} else if r.slow == nil {
			result.SlowSQL.Skipped = "source_unavailable"
			result.SlowSQL.Reason = "unavailable"
		} else {
			value, err := r.slow.Prune(ctx, options)
			result.SlowSQL = TypePruneResult{PruneResult: value}
			failure = errors.Join(failure, err)
		}
	}
	return result, failure
}
