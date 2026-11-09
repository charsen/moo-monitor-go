package cloudsync

import (
	"context"
	"errors"
	"fmt"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/records"
	"gitee.com/charsen/moo-monitor-go/slowquery"
)

var ErrDiscardSource = errors.New("local noise discard source unavailable")
var ErrDiscardCloud = errors.New("Cloud local noise discard not confirmed")
var ErrDiscardState = errors.New("local suppression persistence outcome unknown")

type DiscardResult struct {
	CloudConfirmed                                          bool
	Busy                                                    bool
	CloudDeleted                                            int64
	Candidates, LocalSuppressed, ChangedOrPreserved, Failed int
	LocalStateUnknown                                       bool
}

type runtimeDiscardClient interface {
	DiscardLocalNoise(context.Context) (int64, error)
}
type slowDiscardClient interface {
	DiscardSlowLocalNoise(context.Context) (int64, error)
}

// DiscardLocalNoise is explicit. Cloud affects its token's entire local unresolved
// type set; only this scope's exact unconfirmed local open snapshots are suppressed.
func (s *Syncer) DiscardLocalNoise(ctx context.Context) (DiscardResult, error) {
	client, ok := s.client.(runtimeDiscardClient)
	if !ok {
		return DiscardResult{}, ErrDiscardSource
	}
	return s.discard(ctx, client.DiscardLocalNoise, func(record records.Record) bool { return record.Context.Env == "local" })
}
func (s *SlowSyncer) DiscardLocalNoise(ctx context.Context) (DiscardResult, error) {
	adapter, ok := s.client.(slowClient)
	if !ok {
		return DiscardResult{}, ErrDiscardSource
	}
	client, ok := adapter.client.(slowDiscardClient)
	if !ok {
		return DiscardResult{}, ErrDiscardSource
	}
	return s.discard(ctx, client.DiscardSlowLocalNoise, func(record slowquery.Record) bool { return record.Context.Env == "local" })
}
func (s *synchronizer[R]) discard(ctx context.Context, cloud func(context.Context) (int64, error), local func(R) bool) (DiscardResult, error) {
	result := DiscardResult{}
	if ctx == nil {
		return result, errors.New("discard requires context")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	select {
	case <-s.gate:
		defer func() { s.gate <- struct{}{} }()
	default:
		result.Busy = true
		return result, nil
	}
	if s.root == nil {
		return result, errors.New("discard coordinator is closed")
	}
	lock, held, err := s.lock()
	if err != nil {
		return result, err
	}
	if !held {
		result.Busy = true
		return result, nil
	}
	defer lock.Close()
	reader, ok := s.store.(interface {
		ReadOnlySnapshot(context.Context) ([]buffer.VersionedSnapshot[R], error)
	})
	if !ok {
		return result, ErrDiscardSource
	}
	ledger, err := s.load()
	if err != nil {
		return result, err
	}
	suppressed, err := s.loadSuppression()
	if err != nil {
		return result, err
	}
	snapshots, err := reader.ReadOnlySnapshot(ctx)
	if err != nil {
		return result, err
	}
	candidates := []buffer.VersionedSnapshot[R]{}
	for _, snapshot := range snapshots {
		if snapshot.Bucket == "open" && local(snapshot.Record) && pendingSnapshot(snapshot, ledger.Confirmed[snapshot.Hash], suppressed.Entries[snapshot.Hash]) {
			candidates = append(candidates, snapshot)
		} else {
			result.ChangedOrPreserved++
		}
	}
	result.Candidates = len(candidates)
	// Type lock stays held, while the buffer hot lock was released by the reader.
	deleted, err := cloud(ctx)
	if err != nil {
		return result, discardCloudError(err)
	}
	if deleted < 0 {
		return result, ErrDiscardCloud
	}
	result.CloudConfirmed = true
	result.CloudDeleted = deleted
	current, err := reader.ReadOnlySnapshot(ctx)
	if err != nil {
		result.Failed = len(candidates)
		return result, fmt.Errorf("recheck discard candidates: %w", err)
	}
	live := map[string]buffer.VersionedSnapshot[R]{}
	for _, snapshot := range current {
		live[snapshot.Hash] = snapshot
	}
	applied := 0
	for _, candidate := range candidates {
		value, exists := live[candidate.Hash]
		if !exists || value.Bucket != "open" || !local(value.Record) || candidate.Digest != value.Digest || candidate.Timestamp != value.Timestamp || candidate.Bucket != value.Bucket {
			result.ChangedOrPreserved++
			continue
		}
		suppressed.Entries[candidate.Hash] = suppressSnapshot(candidate)
		applied++
	}
	if applied == 0 {
		return result, nil
	}
	if err = ctx.Err(); err != nil {
		result.Failed = applied
		return result, err
	}
	if err = s.saveSuppression(suppressed); err != nil {
		result.Failed = applied
		result.LocalStateUnknown = true
		return result, errors.Join(ErrDiscardState, err)
	}
	result.LocalSuppressed = applied
	return result, nil
}
func discardCloudError(err error) error {
	for _, category := range []error{context.Canceled, context.DeadlineExceeded, ErrHTTPConfig, ErrWire, ErrHTTPTransport, ErrHTTPStatus, ErrHTTPReceipt, ErrResponseTooLarge} {
		if errors.Is(err, category) {
			return errors.Join(ErrDiscardCloud, category)
		}
	}
	return ErrDiscardCloud
}
