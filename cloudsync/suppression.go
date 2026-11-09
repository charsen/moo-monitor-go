package cloudsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"time"
	"unicode/utf8"

	"gitee.com/charsen/moo-monitor-go/buffer"
)

const runtimeSuppressionFile = "suppression-runtimes.json"
const slowSuppressionFile = "suppression-slow-queries.json"

// This is intentional local suppression, never an intake acknowledgement.
type suppressedVersion struct {
	Digest    string `json:"digest"`
	Timestamp string `json:"timestamp"`
	Bucket    string `json:"bucket"`
}
type suppressionLedger struct {
	Version int                          `json:"version"`
	Entries map[string]suppressedVersion `json:"entries"`
}

func suppressionFile(stateName string) (string, error) {
	switch stateName {
	case stateFile:
		return runtimeSuppressionFile, nil
	case slowStateFile:
		return slowSuppressionFile, nil
	}
	return "", errors.New("unknown suppression namespace")
}
func decodeSuppression(data []byte) (suppressionLedger, error) {
	invalid := errors.New("invalid local suppression ledger")
	if !utf8.Valid(data) || !uniqueStateKeys(data) {
		return suppressionLedger{}, invalid
	}
	var fields map[string]json.RawMessage
	if decodeJSON(data, &fields) != nil || len(fields) != 2 {
		return suppressionLedger{}, invalid
	}
	for _, name := range []string{"version", "entries"} {
		raw, present := fields[name]
		if !present || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return suppressionLedger{}, invalid
		}
	}
	var ledger suppressionLedger
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&ledger) != nil || ledger.Version != 1 || ledger.Entries == nil {
		return suppressionLedger{}, invalid
	}
	var rawEntries map[string]map[string]json.RawMessage
	if json.Unmarshal(fields["entries"], &rawEntries) != nil {
		return suppressionLedger{}, invalid
	}
	for hash, value := range ledger.Entries {
		if !hashPattern.MatchString(hash) || !digestPattern.MatchString(value.Digest) || value.Bucket != "open" {
			return suppressionLedger{}, invalid
		}
		raw := rawEntries[hash]
		if len(raw) != 3 {
			return suppressionLedger{}, invalid
		}
		for _, name := range []string{"digest", "timestamp", "bucket"} {
			field, present := raw[name]
			if !present || bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
				return suppressionLedger{}, invalid
			}
		}
		if _, err := time.Parse(time.RFC3339Nano, value.Timestamp); err != nil {
			return suppressionLedger{}, invalid
		}
	}
	return ledger, nil
}
func (s *synchronizer[R]) loadSuppression() (suppressionLedger, error) {
	name, err := suppressionFile(s.stateName)
	if err != nil {
		return suppressionLedger{}, err
	}
	if err = s.safe(name, true); err != nil {
		return suppressionLedger{}, err
	}
	data, err := s.root.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return suppressionLedger{Version: 1, Entries: map[string]suppressedVersion{}}, nil
	}
	if err != nil {
		return suppressionLedger{}, errors.New("cannot read local suppression ledger")
	}
	return decodeSuppression(data)
}
func suppressionMatches[R any](value suppressedVersion, snapshot buffer.VersionedSnapshot[R]) bool {
	return value.Bucket == "open" && snapshot.Bucket == "open" && value.Digest == snapshot.Digest && value.Timestamp == snapshot.Timestamp
}
func suppressSnapshot[R any](snapshot buffer.VersionedSnapshot[R]) suppressedVersion {
	return suppressedVersion{Digest: snapshot.Digest, Timestamp: snapshot.Timestamp, Bucket: snapshot.Bucket}
}

// pendingSnapshot is shared by send selection, exact statistics and explicit discard.
// Classification still checks genuine confirmed first; suppression is not an ACK.
func pendingSnapshot[R any](snapshot buffer.VersionedSnapshot[R], confirmed Confirmed, suppressed suppressedVersion) bool {
	return !matches(confirmed, snapshot) && !suppressionMatches(suppressed, snapshot)
}

func (s *synchronizer[R]) saveSuppression(value suppressionLedger) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err = decodeSuppression(data); err != nil {
		return err
	}
	name, err := suppressionFile(s.stateName)
	if err != nil {
		return err
	}
	return s.saveBytes(name, data)
}
