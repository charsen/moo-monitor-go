package buffer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
)

var ErrImportConflict = errors.New("import conflicts with existing buffer data")

const ImportJournalName = "import-intent.json"

type ImportOptions struct{ OwnerSeal string }
type ImportResult struct{ Created, AlreadyPresent bool }
type ImportJournal struct {
	Version int    `json:"version"`
	Owner   string `json:"owner"`
	Hash    string `json:"hash"`
	Bucket  string `json:"bucket"`
	Digest  string `json:"digest"`
}

// DecodeImportJournal validates the fixed ownership image without recovering or writing it.
func DecodeImportJournal(data []byte) (ImportJournal, error) {
	bad := ErrImportConflict
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return ImportJournal{}, bad
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return ImportJournal{}, bad
		}
		name, ok := key.(string)
		if !ok || fields[name] != nil {
			return ImportJournal{}, bad
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return ImportJournal{}, bad
		}
		fields[name] = value
	}
	if _, err = decoder.Token(); err != nil {
		return ImportJournal{}, bad
	}
	if _, err = decoder.Token(); err != io.EOF {
		return ImportJournal{}, bad
	}
	for _, key := range []string{"version", "owner", "hash", "bucket", "digest"} {
		if fields[key] == nil {
			return ImportJournal{}, bad
		}
	}
	if len(fields) != 5 {
		return ImportJournal{}, bad
	}
	canonical, err := json.Marshal(fields)
	if err != nil {
		return ImportJournal{}, bad
	}
	strict := json.NewDecoder(bytes.NewReader(canonical))
	strict.DisallowUnknownFields()
	var journal ImportJournal
	if strict.Decode(&journal) != nil || journal.Version != 1 || !validDigest.MatchString(journal.Owner) || !validDigest.MatchString(journal.Digest) || !validHash.MatchString(journal.Hash) || !knownBucket(journal.Bucket) {
		return ImportJournal{}, bad
	}
	return journal, nil
}

// Import writes a typed final image without Build/Refresh or counter increments.
// Only the matching owner and final-image digest may resume its private journal.
func (b *store[R, S, V]) Import(ctx context.Context, record R, options ImportOptions) (ImportResult, error) {
	result := ImportResult{}
	if !validDigest.MatchString(options.OwnerSeal) {
		return result, ErrImportConflict
	}
	data, err := b.builder.Encode(record)
	if err != nil {
		return result, err
	}
	state := b.builder.State(record)
	if !validHash.MatchString(state.Hash) || !knownBucket(state.Status) {
		return result, ErrImportConflict
	}
	if _, err = b.builder.Decode(data, state.Hash); err != nil {
		return result, err
	}
	if err = b.enter(ctx); err != nil {
		return result, err
	}
	defer b.leave()
	lock, err := b.lock(ctx)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	if _, exists, err := b.read("intent.yaml"); err != nil {
		return result, err
	} else if exists {
		return result, ErrRecoveryRequired
	}
	if _, exists, err := b.read("overflow/" + state.Hash + ".yaml"); err != nil {
		return result, err
	} else if exists {
		return result, ErrImportConflict
	}
	expected := ImportJournal{Version: 1, Owner: options.OwnerSeal, Hash: state.Hash, Bucket: state.Status, Digest: contentDigest(data)}
	journal, exists, err := b.read(ImportJournalName)
	if err != nil {
		return result, err
	}
	otherJournal := false
	if exists {
		actual, err := DecodeImportJournal(journal)
		otherJournal = actual.Hash != expected.Hash
		if err != nil || actual.Owner != expected.Owner || (!otherJournal && actual != expected) {
			return result, ErrImportConflict
		}
	}
	found := false
	for _, bucket := range buckets {
		existing, present, err := b.read(bucket + "/" + state.Hash + ".yaml")
		if err != nil {
			return result, err
		}
		if !present {
			continue
		}
		if found || bucket != state.Status || !bytes.Equal(existing, data) {
			return result, ErrImportConflict
		}
		found = true
	}
	if otherJournal {
		if found {
			result.AlreadyPresent = true
			return result, nil
		}
		return result, ErrImportConflict
	}
	if !found {
		if !exists {
			journal, err = json.Marshal(expected)
			if err != nil {
				return result, err
			}
			if err = b.atomicWrite(ImportJournalName, journal); err != nil {
				return result, errors.Join(ErrOutcomeUnknown, err)
			}
		}
		if err = b.atomicWrite(state.Status+"/"+state.Hash+".yaml", data); err != nil {
			return result, errors.Join(ErrOutcomeUnknown, err)
		}
		result.Created = true
	} else {
		result.AlreadyPresent = true
	}
	if !found || exists {
		if err = b.remove(ImportJournalName); err != nil {
			return result, errors.Join(ErrOutcomeUnknown, err)
		}
	}
	return result, nil
}
