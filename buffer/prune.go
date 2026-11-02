package buffer

import (
	"context"
	"errors"
)

// PruneResolved removes only an exact resolved version, and never recovers a journal.
func (b *store[R, S, V]) PruneResolved(ctx context.Context, snapshot VersionedSnapshot[R]) (bool, error) {
	if snapshot.Bucket != "resolved" || !validHash.MatchString(snapshot.Hash) || !validDigest.MatchString(snapshot.Digest) {
		return false, errors.New("prune requires an exact resolved snapshot")
	}
	if err := b.enter(ctx); err != nil {
		return false, err
	}
	defer b.leave()
	lock, err := b.lock(ctx)
	if err != nil {
		return false, err
	}
	defer lock.Close()
	if _, exists, err := b.read("intent.yaml"); err != nil {
		return false, err
	} else if exists {
		return false, ErrRecoveryRequired
	}
	return b.compareLocked(snapshot, Decision{Disposition: Confirm})
}
