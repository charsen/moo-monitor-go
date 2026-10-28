package buffer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"time"

	"gitee.com/charsen/moo-go-monitor/records"
)

type Snapshot struct {
	Hash      string
	Bucket    string
	Timestamp string
	Digest    string
	Record    records.Record
}

type Disposition string

const (
	Confirm    Disposition = "confirm"
	Quarantine Disposition = "quarantine"
)

type Decision struct {
	Disposition Disposition
	Reason      string
}

// ScopeRoot exposes the fixed scope location and anchored inode to the real sync consumer.
func (b *Buffer) ScopeRoot() (string, os.FileInfo, error) {
	if err := b.enter(context.Background()); err != nil {
		return "", nil, err
	}
	defer b.leave()
	if b.root == nil {
		return "", nil, errors.New("buffer is closed")
	}
	info, err := b.root.Stat(".")
	if err != nil {
		return "", nil, errors.New("cannot inspect buffer scope root")
	}
	return b.path, info, nil
}

// Snapshot reads immutable values under the hot lock, then releases it before any transport call.
func (b *Buffer) Snapshot(ctx context.Context) ([]Snapshot, error) {
	if err := b.enter(ctx); err != nil {
		return nil, err
	}
	defer b.leave()
	lock, err := b.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err = b.recover(); err != nil {
		return nil, err
	}
	snapshots := []Snapshot{}
	seen := map[string]bool{}
	for _, bucket := range []string{"open", "resolved"} {
		hashes, err := b.entries(bucket)
		if err != nil {
			return nil, err
		}
		for _, hash := range hashes {
			if seen[hash] {
				return nil, errors.New("duplicate live buffer hash across buckets")
			}
			seen[hash] = true
			data, exists, err := b.read(bucket + "/" + hash + ".yaml")
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, errors.New("buffer snapshot disappeared while locked")
			}
			record, err := decodeRecord(data, hash)
			if err != nil {
				return nil, err
			}
			timestamp, err := recordTimestamp(*record)
			if err != nil {
				return nil, err
			}
			record.Status = bucket
			snapshots = append(snapshots, Snapshot{Hash: hash, Bucket: bucket, Timestamp: timestamp, Digest: contentDigest(data), Record: *record})
		}
	}
	sort.Slice(snapshots, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339Nano, snapshots[i].Timestamp)
		right, _ := time.Parse(time.RFC3339Nano, snapshots[j].Timestamp)
		if left.Equal(right) {
			return snapshots[i].Hash < snapshots[j].Hash
		}
		return left.Before(right)
	})
	return snapshots, nil
}

func recordTimestamp(record records.Record) (string, error) {
	timestamp := record.LastSeen
	if value, exists := record.Meta["updated_at"]; exists && value != nil {
		text, ok := value.(string)
		if !ok {
			return "", errors.New("invalid snapshot timestamp type")
		}
		timestamp = text
	}
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		return "", errors.New("invalid snapshot timestamp")
	}
	return timestamp, nil
}
func contentDigest(data []byte) string {
	value := sha256.Sum256(data)
	return hex.EncodeToString(value[:])
}

var validDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// SafeRejectionReason keeps proven protocol codes only; arbitrary Cloud text is not diagnostic data.
func SafeRejectionReason(reason string) string {
	switch reason {
	case "invalid_record", "ingest_filter", "upsert_failed":
		return reason
	}
	return "rejected"
}

type rejectionReceipt struct {
	Version   int    `json:"version"`
	Hash      string `json:"hash"`
	Digest    string `json:"digest"`
	Status    string `json:"status"`
	Retryable bool   `json:"retryable"`
	Reason    string `json:"reason"`
}

// Compare rechecks actual storage bytes. A stale acknowledgement never removes a newer version.
func (b *Buffer) Compare(ctx context.Context, snapshot Snapshot, decision Decision) (bool, error) {
	if err := b.enter(ctx); err != nil {
		return false, err
	}
	defer b.leave()
	if !validHash.MatchString(snapshot.Hash) || !validDigest.MatchString(snapshot.Digest) || (snapshot.Bucket != "open" && snapshot.Bucket != "resolved") {
		return false, errors.New("invalid sync snapshot identity")
	}
	if decision.Disposition != Confirm && decision.Disposition != Quarantine {
		return false, errors.New("invalid snapshot disposition")
	}
	lock, err := b.lock(ctx)
	if err != nil {
		return false, err
	}
	defer lock.Close()
	if err = b.recover(); err != nil {
		return false, err
	}
	source := snapshot.Bucket + "/" + snapshot.Hash + ".yaml"
	data, exists, err := b.read(source)
	if err != nil || !exists {
		return false, err
	}
	record, err := decodeRecord(data, snapshot.Hash)
	if err != nil {
		return false, err
	}
	timestamp, err := recordTimestamp(*record)
	if err != nil {
		return false, err
	}
	if timestamp != snapshot.Timestamp || contentDigest(data) != snapshot.Digest {
		return false, nil
	}
	if decision.Disposition == Confirm {
		if snapshot.Bucket == "resolved" {
			if err = b.remove(source); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	if err = ensureDirectory(b.root, "cloud-rejected"); err != nil {
		return false, err
	}
	base := "cloud-rejected/" + snapshot.Hash + "-" + snapshot.Digest
	receipt, err := json.Marshal(rejectionReceipt{Version: 1, Hash: snapshot.Hash, Digest: snapshot.Digest, Status: "skipped", Reason: SafeRejectionReason(decision.Reason)})
	if err != nil {
		return false, errors.New("cannot encode rejection receipt")
	}
	// These two local files are not a cloud transaction. Retry verifies each completed half.
	for _, target := range []struct {
		name string
		data []byte
	}{{base + ".yaml", data}, {base + ".receipt.json", receipt}} {
		stored, exists, err := b.read(target.name)
		if err != nil {
			return false, err
		}
		if exists {
			if !bytes.Equal(stored, target.data) {
				return false, errors.New("quarantine content conflict; source retained")
			}
		} else {
			if err = b.atomicWrite(target.name, target.data); err != nil {
				return false, fmt.Errorf("persist quarantine before removal: %w", err)
			}
		}
		stored, exists, err = b.read(target.name)
		if err != nil || !exists || !bytes.Equal(stored, target.data) {
			return false, errors.New("cannot verify quarantine; source retained")
		}
	}
	if err = b.remove(source); err != nil {
		return false, err
	}
	return true, nil
}
