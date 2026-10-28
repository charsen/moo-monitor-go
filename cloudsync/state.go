package cloudsync

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
	"unicode/utf8"

	"gitee.com/charsen/moo-go-monitor/buffer"
	"github.com/gofrs/flock"
)

const stateFile = "sync-runtimes.json"
const lockFile = ".sync-runtimes.lock"

type Confirmed struct {
	Digest    string `json:"digest"`
	Timestamp string `json:"timestamp"`
	Bucket    string `json:"bucket"`
}
type state struct {
	Version   int                  `json:"version"`
	Cursor    string               `json:"cursor"`
	Confirmed map[string]Confirmed `json:"confirmed"`
}

var hashPattern = regexp.MustCompile(`^[a-f0-9]{12}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func openStateRoot(store *buffer.Buffer) (*os.Root, string, error) {
	path, anchored, err := store.ScopeRoot()
	if err != nil {
		return nil, "", err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || !os.SameFile(anchored, info) {
		return nil, "", errors.New("unsafe sync scope root")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, "", errors.New("cannot open sync scope root")
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(anchored, opened) {
		root.Close()
		return nil, "", errors.New("sync scope identity changed")
	}
	return root, path, nil
}

func (s *Syncer) safe(name string, missing bool) error {
	info, err := s.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) && missing {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("unsafe sync state file")
	}
	return nil
}
func (s *Syncer) prepareLock() error {
	if err := s.safe(lockFile, true); err != nil {
		return err
	}
	file, err := s.root.OpenFile(lockFile, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return s.safe(lockFile, false)
	}
	if err != nil {
		return errors.New("cannot create sync lock")
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	return s.syncDirectory()
}
func (s *Syncer) lock() (*flock.Flock, bool, error) {
	if err := s.safe(lockFile, false); err != nil {
		return nil, false, err
	}
	lock := flock.New(filepath.Join(s.path, lockFile), flock.SetFlag(syncLockFlags()), flock.SetPermissions(0600))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		lock.Close()
		return nil, ok, err
	}
	held, err := lock.Stat()
	rooted, rootErr := s.root.Stat(lockFile)
	if err != nil || rootErr != nil || !os.SameFile(held, rooted) {
		lock.Close()
		return nil, false, errors.New("sync lock identity changed")
	}
	return lock, true, nil
}
func (s *Syncer) load() (state, error) {
	empty := state{Version: 1, Confirmed: map[string]Confirmed{}}
	if err := s.safe(stateFile, true); err != nil {
		return state{}, err
	}
	data, err := s.root.ReadFile(stateFile)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return state{}, errors.New("cannot read sync state")
	}
	if !utf8.Valid(data) || !uniqueStateKeys(data) {
		return state{}, errors.New("invalid sync state UTF-8")
	}
	var fields map[string]json.RawMessage
	if err := decodeJSON(data, &fields); err != nil || len(fields) != 3 {
		return state{}, errors.New("invalid sync state")
	}
	for _, key := range []string{"version", "cursor", "confirmed"} {
		raw, exists := fields[key]
		if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return state{}, errors.New("incomplete sync state")
		}
	}
	var result state
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&result); err != nil || result.Version != 1 || result.Confirmed == nil {
		return state{}, errors.New("unsupported sync state")
	}
	if result.Cursor != "" {
		if _, err = time.Parse(time.RFC3339Nano, result.Cursor); err != nil {
			return state{}, errors.New("invalid sync cursor")
		}
	}
	var rawConfirmed map[string]map[string]json.RawMessage
	if err = json.Unmarshal(fields["confirmed"], &rawConfirmed); err != nil {
		return state{}, errors.New("invalid confirmed ledger")
	}
	for hash, value := range result.Confirmed {
		if !hashPattern.MatchString(hash) || !digestPattern.MatchString(value.Digest) || (value.Bucket != "open" && value.Bucket != "resolved") {
			return state{}, errors.New("invalid confirmed identity")
		}
		raw := rawConfirmed[hash]
		if len(raw) != 3 {
			return state{}, errors.New("incomplete confirmed version")
		}
		for _, name := range []string{"digest", "timestamp", "bucket"} {
			field, exists := raw[name]
			if !exists || bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
				return state{}, errors.New("incomplete confirmed version")
			}
		}
		if _, err = time.Parse(time.RFC3339Nano, value.Timestamp); err != nil {
			return state{}, errors.New("invalid confirmed timestamp")
		}
	}
	return result, nil
}

func (s *Syncer) save(value state) error {
	data, err := json.Marshal(value)
	if err != nil {
		return errors.New("cannot encode sync state")
	}
	if err = s.safe(stateFile, true); err != nil {
		return err
	}
	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		return err
	}
	temp := stateFile + ".tmp-" + hex.EncodeToString(suffix[:])
	file, err := s.root.OpenFile(temp, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("cannot create sync state temporary file")
	}
	defer func() { file.Close(); s.root.Remove(temp) }()
	var n int
	if s.writeTest != nil {
		n, err = s.writeTest(file, data)
	} else {
		n, err = file.Write(data)
	}
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return fmt.Errorf("incomplete sync state write: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("cannot sync state file: %w", err)
	}
	if err = file.Close(); err != nil {
		return err
	}
	if s.fault != nil {
		if err = s.fault("replace-state"); err != nil {
			return err
		}
	}
	if err = s.root.Rename(temp, stateFile); err != nil {
		return fmt.Errorf("cannot replace sync state: %w", err)
	}
	return s.syncDirectory()
}
func (s *Syncer) syncDirectory() error {
	if s.fault != nil {
		if err := s.fault("sync-directory"); err != nil {
			return err
		}
	}
	dir, err := s.root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// State has one private schema. Duplicate keys must not replace an earlier confirmed image.
func uniqueStateKeys(data []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 8 {
			return false
		}
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		delim, container := token.(json.Delim)
		if !container {
			return true
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return false
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return false
				}
				seen[name] = true
				if !value(depth + 1) {
					return false
				}
			}
			end, err := decoder.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			for decoder.More() {
				if !value(depth + 1) {
					return false
				}
			}
			end, err := decoder.Token()
			return err == nil && end == json.Delim(']')
		}
		return false
	}
	if !value(0) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}
