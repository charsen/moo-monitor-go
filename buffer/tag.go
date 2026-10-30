package buffer

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// TagSource upgrades metadata atomically without recording another occurrence or reopening a bucket.
func (b *Buffer) TagSource(ctx context.Context, hash, source string, meta map[string]any, now time.Time) (bool, error) {
	if err := b.enter(ctx); err != nil {
		return false, err
	}
	defer b.leave()
	if !validHash.MatchString(hash) {
		return false, errors.New("invalid buffer hash")
	}
	lock, err := b.lock(ctx)
	if err != nil {
		return false, err
	}
	defer lock.Close()
	if err = b.recover(); err != nil {
		return false, err
	}
	existing, bucket, err := b.find(hash)
	if err != nil || existing == nil {
		return false, err
	}
	final, changed, err := b.builder.TagMetadata(*existing, source, meta)
	if err != nil {
		return false, err
	}
	if !changed {
		return true, nil
	}
	if b.dailyCap > 0 && existing.Daily.Date == now.Format("2006-01-02") && existing.Daily.Count >= int64(b.dailyCap) {
		return true, nil
	}
	overflow, err := b.readOverflow(hash)
	if err != nil {
		return false, err
	}
	// Buckets are authoritative even for an older record with a stale status field.
	final.Status = bucket
	final.Meta["updated_at"] = iso(now)
	change := intent{Version: 1, Hash: hash, Target: bucket, Record: final, Overflow: overflow}
	if err = b.writeIntent(change); err != nil {
		return false, fmt.Errorf("%w: persist tag intent: %w", ErrOutcomeUnknown, err)
	}
	if err = b.check("tag-intent"); err != nil {
		return false, fmt.Errorf("%w: tag intent: %w", ErrOutcomeUnknown, err)
	}
	if err = b.writeRecord(bucket, *final); err != nil {
		return false, fmt.Errorf("%w: write tag target: %w", ErrOutcomeUnknown, err)
	}
	if err = b.check("tag-target"); err != nil {
		return false, fmt.Errorf("%w: tag target: %w", ErrOutcomeUnknown, err)
	}
	for _, other := range buckets {
		if other != bucket {
			if err = b.remove(other + "/" + hash + ".yaml"); err != nil {
				return false, fmt.Errorf("%w: remove duplicate tag bucket: %w", ErrOutcomeUnknown, err)
			}
		}
	}
	if err = b.check("tag-old-removed"); err != nil {
		return false, fmt.Errorf("%w: tag old removed: %w", ErrOutcomeUnknown, err)
	}
	if err = b.writeOverflow(hash, overflow); err != nil {
		return false, fmt.Errorf("%w: tag overflow: %w", ErrOutcomeUnknown, err)
	}
	if err = b.remove("intent.yaml"); err != nil {
		return false, fmt.Errorf("%w: clear tag intent: %w", ErrOutcomeUnknown, err)
	}
	return true, nil
}
