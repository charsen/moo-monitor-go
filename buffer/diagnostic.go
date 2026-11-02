package buffer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"time"

	"gitee.com/charsen/moo-go-monitor/records"
	"go.yaml.in/yaml/v3"
)

var ErrDiagnosticConflict = errors.New("diagnostic import ownership conflict")
var ErrReceiptLimit = errors.New("diagnostic receipt budget exhausted")
var artifactPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

const receiptDirectory = "diagnostic-receipts"

type DiagnosticIdentity struct{ ArtifactID, RawDigest string }
type DiagnosticInput struct {
	Identity    DiagnosticIdentity
	Snapshot    records.Snapshot
	OccurredAt  time.Time
	MaxReceipts int
}
type DiagnosticResult struct {
	Hash, ImageDigest         string
	Imported, AlreadyImported bool
}
type diagnosticReceipt struct {
	Version     int    `json:"version"`
	ArtifactID  string `json:"artifact_id"`
	RawDigest   string `json:"raw_digest"`
	Hash        string `json:"hash"`
	ImageDigest string `json:"image_digest"`
}
type diagnosticIntent struct {
	Version     int       `yaml:"version"`
	ArtifactID  string    `yaml:"artifact_id"`
	RawDigest   string    `yaml:"raw_digest"`
	Hash        string    `yaml:"hash"`
	ImageDigest string    `yaml:"image_digest"`
	Target      string    `yaml:"target"`
	Record      yaml.Node `yaml:"record"`
}

// ImportDiagnostic prepares one diagnostic event only after consulting permanent
// receipts. It never refreshes an unrelated existing record or bypasses MaxOpen.
func (b *Buffer) ImportDiagnostic(ctx context.Context, input DiagnosticInput) (DiagnosticResult, error) {
	var result DiagnosticResult
	if !artifactPattern.MatchString(input.Identity.ArtifactID) || !validDigest.MatchString(input.Identity.RawDigest) {
		return result, ErrDiagnosticConflict
	}
	limit := input.MaxReceipts
	if limit == 0 {
		limit = 10000
	}
	if limit < 1 || limit > 100000 {
		return result, ErrDiagnosticConflict
	}
	if err := b.enter(ctx); err != nil {
		return result, err
	}
	defer b.leave()
	lock, err := b.lock(ctx)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	if err = b.recover(); err != nil {
		return result, err
	}
	if _, present, err := b.read(ImportJournalName); err != nil {
		return result, err
	} else if present {
		return result, ErrRecoveryRequired
	}
	receipt, present, err := b.loadDiagnosticReceipt(input.Identity.ArtifactID)
	if err != nil {
		return result, err
	}
	if present {
		if receipt.RawDigest != input.Identity.RawDigest {
			return result, ErrDiagnosticConflict
		}
		return DiagnosticResult{Hash: receipt.Hash, ImageDigest: receipt.ImageDigest, AlreadyImported: true}, nil
	}
	if err = b.diagnosticBudget(limit); err != nil {
		return result, err
	}
	open, err := b.count("open")
	if err != nil {
		return result, err
	}
	if open >= b.maxOpen {
		return result, ErrFull
	}
	snapshot := input.Snapshot
	snapshot.Exception = records.Exception{Class: "CrashDiagnostic", Message: "Unverified crash diagnostic", File: "crash-output/" + input.Identity.ArtifactID}
	snapshot.Source = "crash_diagnostic"
	snapshot.Meta = map[string]any{"diagnostic_artifact_id": input.Identity.ArtifactID, "diagnostic_raw_digest": input.Identity.RawDigest, "diagnostic_may_be_partial": true}
	record, err := b.builder.Build(snapshot, input.OccurredAt)
	if err != nil {
		return result, err
	}
	if existing, _, err := b.find(record.Hash); err != nil {
		return result, err
	} else if existing != nil {
		return result, ErrDiagnosticConflict
	}
	if _, present, err := b.read("overflow/" + record.Hash + ".yaml"); err != nil {
		return result, err
	} else if present {
		return result, ErrDiagnosticConflict
	}
	image, err := EncodeRuntimeRecord(*record)
	if err != nil {
		return result, err
	}
	var document yaml.Node
	if yaml.Unmarshal(image, &document) != nil || len(document.Content) != 1 {
		return result, ErrDiagnosticConflict
	}
	change := diagnosticIntent{Version: 2, ArtifactID: input.Identity.ArtifactID, RawDigest: input.Identity.RawDigest, Hash: record.Hash, ImageDigest: contentDigest(image), Target: "open", Record: *document.Content[0]}
	data, err := yaml.Marshal(change)
	if err != nil {
		return result, ErrDiagnosticConflict
	}
	if err = ensureDirectory(b.root, receiptDirectory); err != nil {
		return result, err
	}
	result.Hash, result.ImageDigest = record.Hash, change.ImageDigest
	if err = b.atomicWrite("intent.yaml", data); err != nil {
		return result, errors.Join(ErrOutcomeUnknown, err)
	}
	if err = b.check("diagnostic-intent-durable"); err != nil {
		return result, errors.Join(ErrOutcomeUnknown, err)
	}
	if err = b.recoverDiagnostic(data); err != nil {
		return result, err
	}
	result.Imported = true
	return result, nil
}
func (b *store[R, S, V]) diagnosticDirectory() (bool, error) {
	info, err := b.root.Lstat(receiptDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return false, ErrDiagnosticConflict
	}
	return true, nil
}
func (b *store[R, S, V]) diagnosticBudget(limit int) error {
	present, err := b.diagnosticDirectory()
	if err != nil || !present {
		return err
	}
	dir, err := b.root.Open(receiptDirectory)
	if err != nil {
		return ErrDiagnosticConflict
	}
	defer dir.Close()
	entries, err := dir.ReadDir(limit + 1)
	if err != nil && err != io.EOF {
		return ErrDiagnosticConflict
	}
	for _, entry := range entries {
		n := entry.Name()
		if entry.IsDir() || len(n) != 37 || n[32:] != ".json" || !artifactPattern.MatchString(n[:32]) {
			return ErrDiagnosticConflict
		}
	}
	if len(entries) >= limit {
		return ErrReceiptLimit
	}
	return nil
}
func (b *store[R, S, V]) loadDiagnosticReceipt(id string) (diagnosticReceipt, bool, error) {
	var receipt diagnosticReceipt
	present, err := b.diagnosticDirectory()
	if err != nil || !present {
		return receipt, false, err
	}
	name := receiptDirectory + "/" + id + ".json"
	info, err := b.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return receipt, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() > 2048 {
		return receipt, false, ErrDiagnosticConflict
	}
	data, present, err := b.read(name)
	if err != nil || !present {
		return receipt, false, ErrDiagnosticConflict
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return receipt, false, ErrDiagnosticConflict
	}
	seen := map[string]bool{}
	allowed := map[string]bool{"version": true, "artifact_id": true, "raw_digest": true, "hash": true, "image_digest": true}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return receipt, false, ErrDiagnosticConflict
		}
		key, ok := token.(string)
		if !ok || !allowed[key] || seen[key] {
			return receipt, false, ErrDiagnosticConflict
		}
		seen[key] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return receipt, false, ErrDiagnosticConflict
		}
	}
	if _, err = decoder.Token(); err != nil || len(seen) != 5 {
		return receipt, false, ErrDiagnosticConflict
	}
	if _, err = decoder.Token(); err != io.EOF {
		return receipt, false, ErrDiagnosticConflict
	}
	if json.Unmarshal(data, &receipt) != nil || receipt.Version != 1 || receipt.ArtifactID != id || !artifactPattern.MatchString(receipt.ArtifactID) || !validDigest.MatchString(receipt.RawDigest) || !validHash.MatchString(receipt.Hash) || !validDigest.MatchString(receipt.ImageDigest) {
		return receipt, false, ErrDiagnosticConflict
	}
	return receipt, true, nil
}

// recoverDiagnostic is a version-2 variant of the existing fixed-image journal.
// Receipt persistence precedes removal of intent, so later removal of the event
// cannot turn the same raw artifact into a second import.
func (b *store[R, S, V]) recoverDiagnostic(data []byte) error {
	runtimeBuilder, ok := any(b.builder).(runtimeFamily)
	if !ok {
		return ErrDiagnosticConflict
	}
	if _, present, err := b.read(ImportJournalName); err != nil {
		return err
	} else if present {
		return ErrRecoveryRequired
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&document) != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return ErrDiagnosticConflict
	}
	var tail yaml.Node
	if decoder.Decode(&tail) != io.EOF {
		return ErrDiagnosticConflict
	}
	seen := map[string]bool{}
	allowed := map[string]bool{"version": true, "artifact_id": true, "raw_digest": true, "hash": true, "image_digest": true, "target": true, "record": true}
	for i := 0; i < len(document.Content[0].Content); i += 2 {
		key, value := document.Content[0].Content[i], document.Content[0].Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || !allowed[key.Value] || seen[key.Value] {
			return ErrDiagnosticConflict
		}
		seen[key.Value] = true
		switch key.Value {
		case "version":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!int" {
				return ErrDiagnosticConflict
			}
		case "record":
			if value.Kind != yaml.MappingNode || value.Tag != "!!map" {
				return ErrDiagnosticConflict
			}
		default:
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return ErrDiagnosticConflict
			}
		}
	}
	if len(seen) != 7 {
		return ErrDiagnosticConflict
	}
	var change diagnosticIntent
	typed := yaml.NewDecoder(bytes.NewReader(data))
	typed.KnownFields(true)
	if typed.Decode(&change) != nil || change.Version != 2 || !artifactPattern.MatchString(change.ArtifactID) || !validDigest.MatchString(change.RawDigest) || !validHash.MatchString(change.Hash) || !validDigest.MatchString(change.ImageDigest) || change.Target != "open" {
		return ErrDiagnosticConflict
	}
	image, err := yaml.Marshal(&change.Record)
	if err != nil {
		return ErrDiagnosticConflict
	}
	final, err := b.builder.Decode(image, change.Hash)
	if err != nil {
		return ErrDiagnosticConflict
	}
	canonical, err := b.builder.Encode(*final)
	if err != nil || contentDigest(canonical) != change.ImageDigest {
		return ErrDiagnosticConflict
	}
	runtimeRecord, ok := any(*final).(records.Record)
	if !ok || runtimeRecord.Count != 1 || runtimeRecord.Daily.Count != 1 || runtimeRecord.Status != "open" || runtimeRecord.Exception.Class != "CrashDiagnostic" || runtimeRecord.Exception.File != "crash-output/"+change.ArtifactID || runtimeBuilder.builder.Hash(runtimeRecord.Exception) != change.Hash {
		return ErrDiagnosticConflict
	}
	expected := diagnosticReceipt{Version: 1, ArtifactID: change.ArtifactID, RawDigest: change.RawDigest, Hash: change.Hash, ImageDigest: change.ImageDigest}
	actual, present, err := b.loadDiagnosticReceipt(change.ArtifactID)
	if err != nil {
		return err
	}
	if present {
		if actual != expected {
			return ErrDiagnosticConflict
		}
		if err = b.remove("intent.yaml"); err != nil {
			return errors.Join(ErrOutcomeUnknown, err)
		}
		return nil
	}
	found := false
	for _, bucket := range buckets {
		existing, present, err := b.read(bucket + "/" + change.Hash + ".yaml")
		if err != nil {
			return err
		}
		if present {
			if found || bucket != "open" || !bytes.Equal(existing, canonical) {
				return ErrDiagnosticConflict
			}
			found = true
		}
	}
	if _, present, err := b.read("overflow/" + change.Hash + ".yaml"); err != nil {
		return err
	} else if present {
		return ErrDiagnosticConflict
	}
	if err = ensureDirectory(b.root, receiptDirectory); err != nil {
		return errors.Join(ErrOutcomeUnknown, err)
	}
	if !found {
		if err = b.atomicWrite("open/"+change.Hash+".yaml", canonical); err != nil {
			return errors.Join(ErrOutcomeUnknown, err)
		}
	}
	if err = b.check("diagnostic-record-durable"); err != nil {
		return errors.Join(ErrOutcomeUnknown, err)
	}
	encoded, err := json.Marshal(expected)
	if err != nil {
		return ErrDiagnosticConflict
	}
	if err = b.atomicWrite(receiptDirectory+"/"+change.ArtifactID+".json", encoded); err != nil {
		return errors.Join(ErrOutcomeUnknown, err)
	}
	if err = b.check("diagnostic-receipt-durable"); err != nil {
		return errors.Join(ErrOutcomeUnknown, err)
	}
	if err = b.remove("intent.yaml"); err != nil {
		return errors.Join(ErrOutcomeUnknown, err)
	}
	return nil
}
