package cloudsync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/records"
)

type Client interface {
	Send(context.Context, []buffer.Snapshot) (Receipt, error)
}
type Config struct{ BatchSize *int }
type Rejected struct{ Hash, Reason string }
type Result struct {
	Scanned, Changed, Sent, Confirmed, Pending int
	// Remaining counts this round's selected versions without a validated, applied disposition.
	// Pending remains the incremental unconfirmed count; historical ACKs may make it zero during a failed replay.
	Remaining int
	Rejected  []Rejected
	Busy      bool
	Cursor    string
}

var ErrPending = errors.New("snapshots remain pending")

type Syncer struct{ *synchronizer[records.Record] }
type snapshotStore[R any] interface {
	Snapshot(context.Context) ([]buffer.VersionedSnapshot[R], error)
	Compare(context.Context, buffer.VersionedSnapshot[R], buffer.Decision) (bool, error)
	ScopeRoot() (string, os.FileInfo, error)
}
type snapshotClient[R any] interface {
	Send(context.Context, []buffer.VersionedSnapshot[R]) (Receipt, error)
}
type synchronizer[R any] struct {
	gate                chan struct{}
	root                *os.Root
	path                string
	store               snapshotStore[R]
	client              snapshotClient[R]
	batchSize           int
	stateName, lockName string
	fault               func(string) error
	writeTest           func(*os.File, []byte) (int, error)
}

func New(store *buffer.Buffer, client Client, config Config) (*Syncer, error) {
	if store == nil || client == nil {
		return nil, errors.New("sync requires explicit buffer and client")
	}
	common, err := newSynchronizer[records.Record](store, client, config, stateFile, lockFile)
	if err != nil {
		return nil, err
	}
	return &Syncer{common}, nil
}
func newSynchronizer[R any](store snapshotStore[R], client snapshotClient[R], config Config, stateName, lockName string) (*synchronizer[R], error) {
	if store == nil || client == nil {
		return nil, errors.New("sync requires explicit buffer and client")
	}
	syncer, err := newStateAccess[R](store, stateName, lockName)
	if err != nil {
		return nil, err
	}
	syncer.client = client
	if config.BatchSize != nil {
		syncer.batchSize = max(1, *config.BatchSize)
	}
	return syncer, nil
}

// newStateAccess is the shared scope/lock owner for Sync and client-free status readers.
func newStateAccess[R any](store snapshotStore[R], stateName, lockName string) (*synchronizer[R], error) {
	root, path, err := openStateRoot(store)
	if err != nil {
		return nil, err
	}
	syncer := &synchronizer[R]{stateName: stateName, lockName: lockName, gate: make(chan struct{}, 1), root: root, path: path, store: store, batchSize: 100}
	syncer.gate <- struct{}{}
	if err = syncer.prepareLock(); err != nil {
		root.Close()
		return nil, err
	}
	return syncer, nil
}
func (s *synchronizer[R]) Close() error {
	<-s.gate
	defer func() { s.gate <- struct{}{} }()
	if s.root == nil {
		return nil
	}
	err := s.root.Close()
	s.root = nil
	return err
}

// Sync never holds the buffer's hot lock during Send. A separate nonblocking lock owns the ledger.
func (s *synchronizer[R]) Sync(ctx context.Context) (Result, error) {
	return s.SyncWithOptions(ctx, SyncOptions{Mode: Incremental})
}
func (s *synchronizer[R]) SyncWithOptions(ctx context.Context, options SyncOptions) (Result, error) {
	result := Result{Rejected: []Rejected{}}
	if ctx == nil || !validSelectionMode(options.Mode) {
		return result, ErrPushConfig
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
		return result, errors.New("syncer is closed")
	}
	lock, ok, err := s.lock()
	if err != nil {
		return result, err
	}
	if !ok {
		result.Busy = true
		return result, nil
	}
	defer lock.Close()
	ledger, err := s.load()
	if err != nil {
		return result, err
	}
	suppressed, err := s.loadSuppression()
	if err != nil {
		return result, err
	}
	snapshots, err := s.store.Snapshot(ctx)
	if err != nil {
		return result, err
	}
	result.Scanned = len(snapshots)
	live := map[string]buffer.VersionedSnapshot[R]{}
	for _, snapshot := range snapshots {
		live[snapshot.Hash] = snapshot
	}
	for hash, value := range ledger.Confirmed {
		current, exists := live[hash]
		if !exists || !matches(value, current) {
			delete(ledger.Confirmed, hash)
		}
	}
	pending := []buffer.VersionedSnapshot[R]{}
	for _, snapshot := range snapshots {
		if selectionReason(snapshot, ledger.Confirmed[snapshot.Hash], suppressed.Entries[snapshot.Hash], options.Mode) == selectionSelected {
			pending = append(pending, snapshot)
		}
	}
	result.Changed = len(pending)
	for _, snapshot := range snapshots {
		if pendingSnapshot(snapshot, ledger.Confirmed[snapshot.Hash], suppressed.Entries[snapshot.Hash]) {
			result.Pending++
		}
	}
	result.Remaining = len(pending)
	ledger.Cursor = safeCursor(ledger.Cursor, snapshots, ledger.Confirmed)
	// Persist pruning / a regressed safe prefix before transport; it confirms no new snapshot.
	if err = s.save(ledger); err != nil {
		return result, err
	}
	result.Cursor = ledger.Cursor
	var pendingCause error
	for start := 0; start < len(pending); start += s.batchSize {
		batch := pending[start:min(start+s.batchSize, len(pending))]
		receipt, err := s.client.Send(ctx, batch)
		result.Sent += len(batch)
		if err != nil {
			return result, fmt.Errorf("send snapshots: %w", err)
		}
		if err = validate(receipt, batch); err != nil {
			return result, err
		}
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
		ledger.Cursor = safeCursor(ledger.Cursor, snapshots, ledger.Confirmed)
		result.Pending = 0
		for _, snapshot := range snapshots {
			if pendingSnapshot(snapshot, ledger.Confirmed[snapshot.Hash], suppressed.Entries[snapshot.Hash]) {
				result.Pending++
			}
		}
		if err = s.save(ledger); err != nil {
			return result, fmt.Errorf("persist acknowledgements: %w", err)
		}
		result.Cursor = ledger.Cursor
	}
	if result.Pending > 0 || result.Remaining > 0 {
		return result, errors.Join(ErrPending, pendingCause)
	}
	return result, nil
}
func matches[R any](value Confirmed, snapshot buffer.VersionedSnapshot[R]) bool {
	return value.Digest == snapshot.Digest && value.Timestamp == snapshot.Timestamp && value.Bucket == snapshot.Bucket
}

// A timestamp group with any hole blocks that group; fingerprints, never time alone, select sends.
func safeCursor[R any](previous string, snapshots []buffer.VersionedSnapshot[R], confirmed map[string]Confirmed) string {
	ordered := append([]buffer.VersionedSnapshot[R]{}, snapshots...)
	sort.Slice(ordered, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339Nano, ordered[i].Timestamp)
		right, _ := time.Parse(time.RFC3339Nano, ordered[j].Timestamp)
		return left.Before(right)
	})
	candidate := previous
	for start := 0; start < len(ordered); {
		timestamp, _ := time.Parse(time.RFC3339Nano, ordered[start].Timestamp)
		end := start
		complete := true
		for end < len(ordered) {
			next, _ := time.Parse(time.RFC3339Nano, ordered[end].Timestamp)
			if !timestamp.Equal(next) {
				break
			}
			if !matches(confirmed[ordered[end].Hash], ordered[end]) {
				complete = false
			}
			end++
		}
		if !complete {
			prior, _ := time.Parse(time.RFC3339Nano, candidate)
			if candidate != "" && !prior.Before(timestamp) {
				candidate = ""
				for _, earlier := range ordered[:start] {
					if matches(confirmed[earlier.Hash], earlier) {
						candidate = earlier.Timestamp
					}
				}
			}
			return candidate
		}
		old, _ := time.Parse(time.RFC3339Nano, candidate)
		if candidate == "" || timestamp.After(old) {
			candidate = ordered[start].Timestamp
		}
		start = end
	}
	return candidate
}
