package cloudsync

import (
	"context"
	"errors"
	"gitee.com/charsen/moo-monitor-go/buffer"
)

// applyValidatedReceipt owns the per-record confirmation rule. The caller must
// validate the entire receipt before entering: a malformed batch may not leave
// even one local side effect. CAS compares the sent fingerprint, so a record
// changed during transport remains pending rather than acknowledging new data.
// Retryable skips, failed comparisons and unknown outcomes do not advance ACKs;
// permanent rejection quarantines only the exact sent version. The caller owns
// cursor recomputation and durable ledger persistence after this step.
func (s *synchronizer[R]) applyValidatedReceipt(ctx context.Context, batch []buffer.VersionedSnapshot[R], receipt Receipt, ledger *state, result *Result) error {
	var pendingCause error
	items := receipt.Results
	if receipt.Legacy {
		items = make([]Item, len(batch))
		for i, snapshot := range batch {
			items[i] = Item{Index: i, Hash: snapshot.Hash, Status: "saved"}
		}
	}
	// The whole batch has passed validation before the first local side effect.
	for _, item := range items {
		snapshot := batch[item.Index]
		decision := buffer.Decision{Disposition: buffer.Confirm}
		permanent := item.Status == "skipped" && !item.Retryable
		if item.Status == "skipped" && item.Retryable {
			pendingCause = errors.Join(pendingCause, ErrPending)
			continue
		}
		if permanent {
			decision.Disposition = buffer.Quarantine
			decision.Reason = buffer.SafeRejectionReason(*item.Reason)
		}
		applied, err := s.store.Compare(ctx, snapshot, decision)
		if err != nil {
			pendingCause = errors.Join(pendingCause, err)
			continue
		}
		if !applied {
			pendingCause = errors.Join(pendingCause, ErrPending)
			continue
		}
		ledger.Confirmed[snapshot.Hash] = Confirmed{Digest: snapshot.Digest, Timestamp: snapshot.Timestamp, Bucket: snapshot.Bucket}
		result.Confirmed++
		result.Remaining--
		if permanent {
			result.Rejected = append(result.Rejected, Rejected{Hash: snapshot.Hash, Reason: decision.Reason})
		}
	}
	return pendingCause
}
