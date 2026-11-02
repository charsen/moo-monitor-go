package cloudsync

import (
	"context"
	"errors"

	"gitee.com/charsen/moo-go-monitor/buffer"
)

type PendingCounts struct {
	Scanned, Open, Resolved          int
	ConfirmedOpen, ConfirmedResolved int
	SuppressedOpen                   int
	PendingOpen, PendingResolved     int
}

// Counts and Cursor are absent when Unknown. A busy/error result is never a known zero.
type PendingStats struct {
	Known, Busy bool
	Counts      *PendingCounts
	Cursor      *string
	Reason      string
}
type readOnlySnapshotStore[R any] interface {
	ReadOnlySnapshot(context.Context) ([]buffer.VersionedSnapshot[R], error)
}

func (s *synchronizer[R]) Stats(ctx context.Context) (PendingStats, error) {
	value, err := s.inspect(ctx, Incremental)
	return value.PendingStats, err
}

// inspect owns one strict snapshot and type lock for statistics and previews.
func (s *synchronizer[R]) inspect(ctx context.Context, mode SelectionMode) (TypePreview, error) {
	result := TypePreview{}
	if !validSelectionMode(mode) {
		return result, ErrPushConfig
	}
	if ctx == nil {
		return result, errors.New("pending statistics require context")
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
		return result, errors.New("syncer is closed")
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
	suppressed, err := s.loadSuppression()
	if err != nil {
		result.Reason = "suppression"
		return result, err
	}
	source, ok := s.store.(readOnlySnapshotStore[R])
	if !ok {
		result.Reason = "unsupported_store"
		return result, errors.New("pending statistics require a read-only snapshot source")
	}
	snapshots, err := source.ReadOnlySnapshot(ctx)
	if err != nil {
		result.Reason = "snapshot"
		if errors.Is(err, buffer.ErrRecoveryRequired) {
			result.Reason = "recovery_required"
		}
		return result, err
	}
	counts := &PendingCounts{Scanned: len(snapshots)}
	selected, excludedConfirmed, excludedSuppressed := 0, 0, 0
	for _, snapshot := range snapshots {
		confirmed := matches(ledger.Confirmed[snapshot.Hash], snapshot)
		switch selectionReason(snapshot, ledger.Confirmed[snapshot.Hash], suppressed.Entries[snapshot.Hash], mode) {
		case selectionSelected:
			selected++
		case selectionSuppressed:
			excludedSuppressed++
		case selectionConfirmed:
			excludedConfirmed++
		}

		if snapshot.Bucket == "open" {
			counts.Open++
			if confirmed {
				counts.ConfirmedOpen++
			} else if pendingSnapshot(snapshot, ledger.Confirmed[snapshot.Hash], suppressed.Entries[snapshot.Hash]) {
				counts.PendingOpen++
			} else {
				counts.SuppressedOpen++
			}
		} else {
			counts.Resolved++
			if confirmed {
				counts.ConfirmedResolved++
			} else {
				counts.PendingResolved++
			}
		}
	}
	result.Known = true
	result.Counts = counts
	result.Cursor = &ledger.Cursor
	result.Selected = &selected
	result.WouldSend = &selected
	result.ExcludedConfirmed = &excludedConfirmed
	result.ExcludedSuppressed = &excludedSuppressed
	return result, nil
}

type TypePendingStats struct {
	PendingStats
	Skipped string
}
type PushPendingStats struct{ Runtimes, SlowSQL TypePendingStats }

// Stats shares type-level synchronization with Push, but never sends data,
// invokes AfterPush, prunes ACKs, recovers journals, or advances a cursor.
func (p *Pusher) Stats(ctx context.Context, target PushTarget) (PushPendingStats, error) {
	result := PushPendingStats{Runtimes: TypePendingStats{Skipped: "not_selected"}, SlowSQL: TypePendingStats{Skipped: "not_selected"}}
	if target != PushBoth && target != PushRuntimes && target != PushSlowSQL {
		return result, ErrPushConfig
	}
	operation, leave, err := p.enterOperation(ctx)
	if err != nil {
		return result, err
	}
	defer leave()
	var failure error
	if target == PushBoth || target == PushRuntimes {
		switch {
		case p.runtime == nil:
			result.Runtimes.Skipped = "source_unavailable"
			failure = errors.Join(failure, ErrPushSource)
		default:
			result.Runtimes.Skipped = ""
			value, err := p.runtime.Stats(operation)
			result.Runtimes.PendingStats = value
			failure = errors.Join(failure, err)
		}
	}
	if target == PushBoth || target == PushSlowSQL {
		switch {
		case p.slow == nil:
			result.SlowSQL.Skipped = "source_unavailable"
			failure = errors.Join(failure, ErrPushSource)
		default:
			result.SlowSQL.Skipped = ""
			value, err := p.slow.Stats(operation)
			result.SlowSQL.PendingStats = value
			failure = errors.Join(failure, err)
		}
	}
	return result, failure
}
