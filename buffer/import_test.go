package buffer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypedImportJournalAndConflict(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store := openBuffer(t, root, "imported", 0, 500)
	record, err := testBuilder(t).Build(testSnapshot(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	record.Count = 7
	record.Daily.Count = 3
	owner := strings.Repeat("a", 64)
	options := ImportOptions{OwnerSeal: owner}
	sentinel := errors.New("fixed import file failure")
	store.fault = func(stage string) error {
		if stage == "rename:open/"+record.Hash+".yaml" {
			return sentinel
		}
		return nil
	}
	result, err := store.Import(context.Background(), *record, options)
	if !errors.Is(err, sentinel) || !errors.Is(err, ErrOutcomeUnknown) || result.Created {
		t.Fatal(result, err)
	}
	journal, err := os.ReadFile(filepath.Join(store.path, ImportJournalName))
	if err != nil {
		t.Fatal("owned intent missing", err)
	}
	decoded, err := DecodeImportJournal(journal)
	if err != nil || decoded.Owner != owner || decoded.Hash != record.Hash {
		t.Fatal(decoded, err)
	}
	store.fault = nil
	result, err = store.Import(context.Background(), *record, options)
	if err != nil || !result.Created {
		t.Fatal("journal did not resume", result, err)
	}
	result, err = store.Import(context.Background(), *record, options)
	if err != nil || !result.AlreadyPresent || result.Created {
		t.Fatal("import increments or rewrites", result, err)
	}
	if actual := getRecord(t, store, record.Hash); actual.Count != 7 || actual.Daily.Count != 3 {
		t.Fatal("import lifecycle reset")
	}
	record.Count++
	if _, err = store.Import(context.Background(), *record, options); !errors.Is(err, ErrImportConflict) {
		t.Fatal("different image replaced", err)
	}
	record.Count--
	record.Status = "deleted"
	if _, err = store.Import(context.Background(), *record, options); !errors.Is(err, ErrImportConflict) {
		t.Fatal("cross bucket accepted", err)
	}
	record.Status = "open"
	wrong, err := json.Marshal(ImportJournal{Version: 1, Owner: strings.Repeat("b", 64), Hash: record.Hash, Bucket: "open", Digest: decoded.Digest})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(store.path, ImportJournalName), wrong, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Import(context.Background(), *record, options); !errors.Is(err, ErrImportConflict) {
		t.Fatal("foreign journal accepted", err)
	}
	actual, err := os.ReadFile(filepath.Join(store.path, ImportJournalName))
	if err != nil || string(actual) != string(wrong) {
		t.Fatal("foreign journal changed")
	}
}
func TestImportJournalStrict(t *testing.T) {
	for _, value := range []string{`null`, `{}`, `{"version":1,"owner":null}`, `{"version":1,"version":1}`, `{"version":1} {}`} {
		if _, err := DecodeImportJournal([]byte(value)); err == nil {
			t.Fatal("bad journal accepted")
		}
	}
}
