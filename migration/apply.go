package migration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/records"
	"gitee.com/charsen/moo-monitor-go/slowquery"
	"github.com/gofrs/flock"
)

var ErrApply = errors.New("migration apply rejected or incomplete")
var ErrApplyBusy = errors.New("migration target is busy")

type ApplyResult struct {
	Created, AlreadyPresent  int
	Complete, OutcomeUnknown bool
}
type importManifest struct {
	Version    int             `json:"version"`
	PlanDigest string          `json:"plan_digest"`
	TargetRoot string          `json:"target_root"`
	Scope      string          `json:"scope"`
	Records    []PlannedRecord `json:"records"`
	Complete   bool            `json:"complete"`
}

// Apply only owns a new isolated namespace, or resumes its exact sealed plan.
// It is a sequence of durable files, never a transaction across source/target files.
func (p *Plan) Apply(ctx context.Context) (result ApplyResult, failure error) {
	if ctx == nil {
		return result, ErrApply
	}
	seal, err := p.seal()
	if err != nil || seal != p.Digest {
		return result, ErrPlan
	}
	if err = p.verifySources(ctx); err != nil {
		return result, err
	}
	root, err := openDirectory(p.options.TargetRoot)
	if err != nil {
		return result, ErrApply
	}
	defer root.Close()
	info, err := root.Stat(".")
	if err != nil || info.Mode().Perm()&0077 != 0 {
		return result, ErrApply
	}
	canonical, err := filepath.EvalSymlinks(p.options.TargetRoot)
	if err != nil {
		return result, ErrApply
	}
	for _, source := range []string{p.sources.RuntimeSource, p.sources.SlowSource} {
		if source != "" {
			sourcePath, err := filepath.EvalSymlinks(source)
			if err != nil || pathsOverlap(canonical, sourcePath) {
				return result, ErrApply
			}
		}
	}
	lockName := ".migration-" + p.options.Scope + ".lock"
	lock, err := lockTarget(root, p.options.TargetRoot, lockName)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	manifestName := ".migration-" + p.options.Scope + ".json"
	expected := importManifest{Version: 1, PlanDigest: p.Digest, TargetRoot: p.options.TargetRoot, Scope: p.options.Scope, Records: p.Records()}
	actual, present, err := readManifest(root, manifestName)
	if err != nil {
		return result, err
	}
	if present {
		if !sameOwnership(actual, expected) {
			return result, ErrApply
		}
		if err = p.scanTarget(root, true, false); err != nil {
			return result, err
		}
	} else {
		if err = p.scanTarget(root, false, false); err != nil {
			return result, err
		}
		if err = writeManifest(root, manifestName, expected, true); err != nil {
			result.OutcomeUnknown = true
			return result, errors.Join(ErrApply, err)
		}
	}
	if err = p.verifySources(ctx); err != nil {
		return result, err
	}
	var runtimeStore *buffer.Buffer
	var slowStore *buffer.SlowBuffer
	defer func() {
		if runtimeStore != nil {
			failure = errors.Join(failure, runtimeStore.Close())
		}
		if slowStore != nil {
			failure = errors.Join(failure, slowStore.Close())
		}
	}()
	for index, entry := range p.records {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if p.fault != nil {
			if err = p.fault("before-record", index); err != nil {
				return result, err
			}
		}
		if err = verifySourceFile(entry.SourceRoot, entry.Relative, entry.SourceDigest); err != nil {
			return result, err
		}
		var imported buffer.ImportResult
		if entry.Family == "runtimes" {
			if runtimeStore == nil {
				builder, err := records.New(records.Config{MaskKeys: p.options.MaskKeys})
				if err != nil {
					return result, err
				}
				runtimeStore, err = buffer.New(buffer.Config{Root: p.options.TargetRoot, Scope: p.options.Scope}, builder)
				if err != nil {
					return result, err
				}
			}
			record, err := buffer.DecodeRuntimeRecord(entry.Data, entry.Hash)
			if err != nil {
				return result, ErrPlan
			}
			imported, err = runtimeStore.Import(ctx, *record, buffer.ImportOptions{OwnerSeal: p.Digest})
		} else {
			if slowStore == nil {
				builder, err := slowquery.New(slowquery.Config{Enabled: true, MaskKeys: p.options.MaskKeys})
				if err != nil {
					return result, err
				}
				slowStore, err = buffer.NewSlow(buffer.Config{Root: p.options.TargetRoot, Scope: p.options.Scope}, builder)
				if err != nil {
					return result, err
				}
			}
			record, err := buffer.DecodeSlowRecord(entry.Data, entry.Hash)
			if err != nil {
				return result, ErrPlan
			}
			imported, err = slowStore.Import(ctx, *record, buffer.ImportOptions{OwnerSeal: p.Digest})
		}
		if err != nil {
			result.OutcomeUnknown = errors.Is(err, buffer.ErrOutcomeUnknown)
			return result, errors.Join(ErrApply, err)
		}
		if imported.Created {
			result.Created++
		}
		if imported.AlreadyPresent {
			result.AlreadyPresent++
		}
	}
	if err = p.verifySources(ctx); err != nil {
		return result, err
	}
	if err = p.scanTarget(root, true, true); err != nil {
		return result, err
	}
	latest, present, err := readManifest(root, manifestName)
	if err != nil || !present || !sameOwnership(latest, expected) {
		return result, ErrApply
	}
	expected.Complete = true
	if !latest.Complete {
		if p.fault != nil {
			if err = p.fault("before-complete", len(p.records)); err != nil {
				return result, err
			}
		}
		if err = writeManifest(root, manifestName, expected, false); err != nil {
			result.OutcomeUnknown = true
			return result, errors.Join(ErrApply, err)
		}
	}
	result.Complete = true
	return result, nil
}
func (p *Plan) verifySources(ctx context.Context) error {
	fresh, err := BuildPlan(ctx, p.sources, p.options)
	if err != nil {
		return err
	}
	if fresh.Digest != p.Digest {
		return ErrPlan
	}
	return nil
}
func verifySourceFile(rootPath, name, expected string) error {
	root, err := openDirectory(rootPath)
	if err != nil {
		return ErrPlan
	}
	defer root.Close()
	data, err := readFile(root, name)
	if err != nil || digest(data) != expected {
		return ErrPlan
	}
	return nil
}
func sameOwnership(a, b importManifest) bool {
	return a.Version == 1 && a.PlanDigest == b.PlanDigest && a.TargetRoot == b.TargetRoot && a.Scope == b.Scope && reflect.DeepEqual(a.Records, b.Records)
}

func (p *Plan) scanTarget(root *os.Root, owned, requireAll bool) error {
	found := 0
	expectedFiles := map[string]string{}
	expectedJournals := map[string]PlannedRecord{}
	for _, entry := range p.records {
		expectedFiles[entry.Family+"/"+entry.Bucket+"/"+entry.Hash+".yaml"] = entry.TargetDigest
		expectedJournals[entry.Family+"/"+entry.Hash] = entry.PlannedRecord
	}
	for _, family := range []string{"runtimes", "slow_sql"} {
		name := "go-runtime-buffer--" + p.options.Scope
		if family == "slow_sql" {
			name = "go-slow-query-buffer--" + p.options.Scope
		}
		info, err := root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			if requireAll {
				for _, entry := range p.records {
					if entry.Family == family {
						return ErrApply
					}
				}
			}
			continue
		}
		if err != nil || !owned || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return ErrApply
		}
		scoped, err := root.OpenRoot(name)
		if err != nil {
			return ErrApply
		}
		actual, err := scoped.Stat(".")
		if err != nil || !os.SameFile(info, actual) {
			scoped.Close()
			return ErrApply
		}
		entries, err := readDirectory(scoped, ".")
		if err != nil {
			scoped.Close()
			return err
		}
		for _, entry := range entries {
			item := entry.Name()
			if item == ".buffer.lock" {
				data, err := readFile(scoped, item)
				if err != nil || len(data) != 0 {
					scoped.Close()
					return ErrApply
				}
				continue
			}
			if item == buffer.ImportJournalName {
				if requireAll {
					scoped.Close()
					return ErrApply
				}
				data, err := readFile(scoped, item)
				if err != nil {
					scoped.Close()
					return ErrApply
				}
				journal, err := buffer.DecodeImportJournal(data)
				if err != nil || journal.Owner != p.Digest {
					scoped.Close()
					return ErrApply
				}
				expected, exists := expectedJournals[family+"/"+journal.Hash]
				matched := exists && expected.Bucket == journal.Bucket && expected.TargetDigest == journal.Digest
				if !matched {
					scoped.Close()
					return ErrApply
				}
				continue
			}
			if item != "open" && item != "resolved" && item != "deleted" && item != "overflow" {
				scoped.Close()
				return ErrApply
			}
			bucketInfo, err := scoped.Lstat(item)
			if err != nil || !bucketInfo.IsDir() || bucketInfo.Mode()&os.ModeSymlink != 0 || bucketInfo.Mode().Perm()&0077 != 0 {
				scoped.Close()
				return ErrApply
			}
			files, err := readDirectory(scoped, item)
			if err != nil {
				scoped.Close()
				return err
			}
			if item == "overflow" && len(files) > 0 {
				scoped.Close()
				return ErrApply
			}
			for _, file := range files {
				relative := filepath.Join(item, file.Name())
				data, err := readFile(scoped, relative)
				if err != nil {
					scoped.Close()
					return ErrApply
				}
				expected, exists := expectedFiles[family+"/"+relative]
				matched := exists && digest(data) == expected
				if !matched {
					scoped.Close()
					return ErrApply
				}
				found++
			}
		}
		scoped.Close()
	}
	if requireAll && found != len(p.records) {
		return ErrApply
	}
	return nil
}
func readDirectory(root *os.Root, name string) ([]os.DirEntry, error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, ErrApply
	}
	defer file.Close()
	entries, err := file.ReadDir(10001)
	if (err != nil && !errors.Is(err, io.EOF)) || len(entries) > 10000 {
		return nil, ErrApply
	}
	return entries, nil
}

func lockTarget(root *os.Root, path, name string) (*flock.Flock, error) {
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		syncErr := file.Sync()
		closeErr := file.Close()
		if syncErr != nil || closeErr != nil {
			return nil, ErrApply
		}
	} else if !errors.Is(err, os.ErrExist) {
		return nil, ErrApply
	}
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != 0 {
		return nil, ErrApply
	}
	lock := flock.New(filepath.Join(path, name), flock.SetFlag(os.O_RDWR), flock.SetPermissions(0600))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		lock.Close()
		return nil, ErrApplyBusy
	}
	held, err := lock.Stat()
	current, currentErr := root.Stat(name)
	if err != nil || currentErr != nil || !os.SameFile(held, current) {
		lock.Close()
		return nil, ErrApply
	}
	return lock, nil
}
func readManifest(root *os.Root, name string) (importManifest, bool, error) {
	data, err := readFile(root, name)
	if errors.Is(err, os.ErrNotExist) {
		return importManifest{}, false, nil
	}
	if err != nil {
		return importManifest{}, false, ErrApply
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return importManifest{}, true, ErrApply
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return importManifest{}, true, ErrApply
		}
		text, ok := key.(string)
		if !ok || fields[text] != nil {
			return importManifest{}, true, ErrApply
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return importManifest{}, true, ErrApply
		}
		fields[text] = value
	}
	if _, err = decoder.Token(); err != nil {
		return importManifest{}, true, ErrApply
	}
	if _, err = decoder.Token(); err != io.EOF {
		return importManifest{}, true, ErrApply
	}
	if len(fields) != 6 {
		return importManifest{}, true, ErrApply
	}
	for _, key := range []string{"version", "plan_digest", "target_root", "scope", "records", "complete"} {
		if fields[key] == nil || bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return importManifest{}, true, ErrApply
		}
	}
	strict := json.NewDecoder(bytes.NewReader(data))
	strict.DisallowUnknownFields()
	var manifest importManifest
	if strict.Decode(&manifest) != nil {
		return importManifest{}, true, ErrApply
	}
	return manifest, true, nil
}
func writeManifest(root *os.Root, name string, value importManifest, create bool) error {
	data, err := json.Marshal(value)
	if err != nil || len(data) > maxBytes {
		return ErrApply
	}
	var token [12]byte
	if _, err = rand.Read(token[:]); err != nil {
		return ErrApply
	}
	temporary := ".migration-tmp-" + hex.EncodeToString(token[:])
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ErrApply
	}
	defer root.Remove(temporary)
	n, writeErr := file.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return ErrApply
	}
	if create {
		err = root.Link(temporary, name)
	} else {
		err = root.Rename(temporary, name)
	}
	if err != nil {
		return ErrApply
	}
	directory, err := root.Open(".")
	if err != nil {
		return ErrApply
	}
	defer directory.Close()
	return directory.Sync()
}
