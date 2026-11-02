package buffer

import (
	"context"
	"errors"
)

var ErrRecoveryRequired = errors.New("buffer has a pending journal; explicit recovery is required")

// ReadOnlySnapshot never replays a journal or writes state. Construction of the
// borrowed Buffer has its own initialization/recovery contract.
func (b *store[R, S, V]) ReadOnlySnapshot(ctx context.Context) ([]VersionedSnapshot[R], error) {
	if err := b.enter(ctx); err != nil {
		return nil, err
	}
	defer b.leave()
	lock, err := b.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if _, exists, err := b.read("intent.yaml"); err != nil {
		return nil, err
	} else if exists {
		return nil, ErrRecoveryRequired
	}
	return b.snapshotLocked()
}
