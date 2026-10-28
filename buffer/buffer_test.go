package buffer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"gitee.com/charsen/moo-go-monitor/mask"
	"gitee.com/charsen/moo-go-monitor/records"
	"go.yaml.in/yaml/v3"
)

var testNow = time.Date(2026, 10, 29, 12, 0, 0, 123000000, time.FixedZone("fixture", 8*3600))

func testSnapshot() records.Snapshot {
	return records.Snapshot{Exception: records.Exception{Class: "BufferFakeException", File: "/__go_moo_buffer_fake_source__/app/fake.php", Line: 17, Message: "fixture failure 123"}, Request: records.Request{Method: "CLI"}, Context: records.GoContext{Env: "testing", Project: "buffer-fixture", Go: "fixture"}, Source: "reportable"}
}
func testBuilder(t *testing.T) *records.Builder {
	t.Helper()
	b, err := records.New(records.Config{MaskKeys: []string{"password", "token"}, BasePath: "/__go_moo_buffer_fake_source__"})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func openBuffer(t *testing.T, root, scope string, cap, max int) *Buffer {
	t.Helper()
	b, err := New(Config{Root: root, Scope: scope, DailyCap: &cap, MaxOpen: &max}, testBuilder(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}
func mustRecord(t *testing.T, b *Buffer, s records.Snapshot, now time.Time) string {
	t.Helper()
	hash, err := b.Record(context.Background(), s, now)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
func getRecord(t *testing.T, b *Buffer, hash string) *records.Record {
	t.Helper()
	r, err := b.Get(context.Background(), hash)
	if err != nil || r == nil {
		t.Fatalf("get err %v", err)
	}
	return r
}
func seedBucket(t *testing.T, b *Buffer, hash, bucket string) {
	t.Helper()
	r := getRecord(t, b, hash)
	r.Status = "open" // Deliberately disagree: the bucket must be authoritative.
	at, by, note := "fixture-at", "fixture-actor", "fixture-note"
	r.ResolvedAt = &at
	r.ResolvedBy = &by
	r.ResolvedNote = &note
	if err := b.writeRecord(bucket, *r); err != nil {
		t.Fatal(err)
	}
	if err := b.remove("open/" + hash + ".yaml"); err != nil {
		t.Fatal(err)
	}
}

func TestBufferLifecycle(t *testing.T) {
	b := openBuffer(t, privateTemp(t), "", 2, 1)
	s := testSnapshot()
	hash := mustRecord(t, b, s, testNow)
	if getRecord(t, b, hash).Count != 1 {
		t.Fatal("new count")
	}
	mustRecord(t, b, s, testNow)
	frozen, err := os.ReadFile(filepath.Join(b.path, "open", hash+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		mustRecord(t, b, s, testNow.Add(time.Duration(i+1)*time.Minute))
	}
	after, _ := os.ReadFile(filepath.Join(b.path, "open", hash+".yaml"))
	if !bytes.Equal(frozen, after) {
		t.Fatal("cap changed frozen bytes")
	}
	if n, err := b.readOverflow(hash); err != nil || n != 3 {
		t.Fatal("overflow", n, err)
	}
	next := testNow.AddDate(0, 0, 1)
	mustRecord(t, b, s, next)
	r := getRecord(t, b, hash)
	if r.Count != 6 || r.Daily.Count != 1 || r.FirstSeen != iso(testNow) || r.Meta["updated_at"] != iso(next) {
		t.Fatalf("backfill %+v", r)
	}
	other := s
	other.Exception.Line++
	if _, err = b.Record(context.Background(), other, next); !errors.Is(err, ErrFull) {
		t.Fatal("max_open", err)
	}
	for _, bucket := range []string{"resolved", "deleted"} {
		seedBucket(t, b, hash, bucket)
		if r := getRecord(t, b, hash); r.Status != bucket {
			t.Fatal("bucket not authoritative")
		}
		mustRecord(t, b, s, next)
		r = getRecord(t, b, hash)
		if r.Status != "open" || r.ResolvedAt != nil || r.ResolvedBy != nil || r.ResolvedNote != nil {
			t.Fatal("recurrence not reopened")
		}
		count, err := b.Count(context.Background(), "all")
		if err != nil || count != 1 {
			t.Fatal("duplicate recurrence", count, err)
		}
	}
	if r.Count != 8 {
		t.Fatal("recurrence count", r.Count)
	}
	rows, err := b.List(context.Background(), "all", 100)
	if err != nil || len(rows) != 1 || rows[0].Hash != hash || rows[0].Count != 8 {
		t.Fatal("list", rows, err)
	}
	if empty, err := b.Get(context.Background(), "aaaaaaaaaaaa"); err != nil || empty != nil {
		t.Fatal("missing", err)
	}
	if _, err := b.Get(context.Background(), "../bad"); err == nil {
		t.Fatal("invalid hash")
	}
}

func TestBufferScopeAndOrdering(t *testing.T) {
	root := privateTemp(t)
	a := openBuffer(t, root, " Alpha / PROJ ", 0, 10)
	b := openBuffer(t, root, "Beta", 0, 10)
	s := testSnapshot()
	hash := mustRecord(t, a, s, testNow)
	if found, err := b.Get(context.Background(), hash); err != nil || found != nil {
		t.Fatal("scope leaked")
	}
	mustRecord(t, b, s, testNow)
	mustRecord(t, a, s, testNow)
	if getRecord(t, a, hash).Count != 2 || getRecord(t, b, hash).Count != 1 {
		t.Fatal("scope counters")
	}
	if filepath.Base(a.path) != "go-runtime-buffer--alpha-proj" {
		t.Fatal("scope", a.path)
	}
	s.Exception.Line++
	s.Exception.Message = "second\nextra"
	second := mustRecord(t, a, s, testNow.Add(time.Hour))
	rows, err := a.List(context.Background(), "all", 100)
	if err != nil || len(rows) != 2 || rows[0].Hash != second || rows[0].MessageFirstLine != "second" {
		t.Fatal("order/firstline", rows, err)
	}
	rows, err = a.List(context.Background(), "all", -1)
	if err != nil || len(rows) != 1 || rows[0].Hash != second {
		t.Fatal("negative array_slice")
	}
	rows, err = a.List(context.Background(), "all", 0)
	if err != nil || len(rows) != 0 {
		t.Fatal("zero limit")
	}
	if _, err = a.Count(context.Background(), "../open"); err == nil {
		t.Fatal("bucket validation")
	}
}

func TestBufferRecovery(t *testing.T) {
	for _, stage := range []string{"intent-durable", "target-durable", "old-removed", "overflow-durable", "remove:intent.yaml"} {
		t.Run(stage, func(t *testing.T) {
			root := privateTemp(t)
			b := openBuffer(t, root, "", 0, 10)
			s := testSnapshot()
			hash := mustRecord(t, b, s, testNow)
			seedBucket(t, b, hash, "resolved")
			b.fault = func(current string) error {
				if current == stage {
					return errors.New("injected interruption")
				}
				return nil
			}
			if _, err := b.Record(context.Background(), s, testNow.Add(time.Hour)); !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatal("expected outcome unknown", err)
			}
			b.Close()
			reopened := openBuffer(t, root, "", 0, 10)
			r := getRecord(t, reopened, hash)
			if r.Count != 2 || r.Status != "open" || r.ResolvedAt != nil {
				t.Fatal("recovered image", r.Count, r.Status)
			}
			count, err := reopened.Count(context.Background(), "all")
			if err != nil || count != 1 {
				t.Fatal("recovered duplicate", err)
			}
			if _, err = os.Stat(filepath.Join(reopened.path, "intent.yaml")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("journal not cleared")
			}
			if getRecord(t, reopened, hash).Count != 2 {
				t.Fatal("read recovery double counted")
			}
		})
	}
	// A frozen update has no record rewrite, but its final overflow must be replayed once.
	for _, stage := range []string{"intent-durable", "overflow-durable", "remove:intent.yaml"} {
		t.Run("frozen/"+stage, func(t *testing.T) {
			root := privateTemp(t)
			b := openBuffer(t, root, "", 1, 10)
			s := testSnapshot()
			hash := mustRecord(t, b, s, testNow)
			original, _ := os.ReadFile(filepath.Join(b.path, "open", hash+".yaml"))
			b.fault = func(current string) error {
				if current == stage {
					return errors.New("injected")
				}
				return nil
			}
			if _, err := b.Record(context.Background(), s, testNow); !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatal(err)
			}
			b.Close()
			reopened := openBuffer(t, root, "", 1, 10)
			getRecord(t, reopened, hash)
			after, _ := os.ReadFile(filepath.Join(reopened.path, "open", hash+".yaml"))
			if !bytes.Equal(original, after) {
				t.Fatal("recovery rewrote frozen record")
			}
			if n, err := reopened.readOverflow(hash); err != nil || n != 1 {
				t.Fatal("frozen overflow", n, err)
			}
			mustRecord(t, reopened, s, testNow.AddDate(0, 0, 1))
			if getRecord(t, reopened, hash).Count != 3 {
				t.Fatal("overflow lost/double counted")
			}
		})
	}
}

func TestBufferIOFailures(t *testing.T) {
	for _, kind := range []string{"short-write", "rename", "remove", "directory-sync"} {
		t.Run(kind, func(t *testing.T) {
			root := privateTemp(t)
			b := openBuffer(t, root, "", 0, 10)
			s := testSnapshot()
			hash := mustRecord(t, b, s, testNow)
			seedBucket(t, b, hash, "deleted")
			switch kind {
			case "short-write":
				calls := 0
				b.writeTest = func(file *os.File, data []byte) (int, error) {
					calls++
					if calls == 2 {
						return file.Write(data[:len(data)/2])
					}
					return file.Write(data)
				}
			case "rename":
				b.fault = func(stage string) error {
					if stage == "rename:open/"+hash+".yaml" {
						return io.ErrUnexpectedEOF
					}
					return nil
				}
			case "remove":
				b.fault = func(stage string) error {
					if stage == "remove:deleted/"+hash+".yaml" {
						return io.ErrUnexpectedEOF
					}
					return nil
				}
			case "directory-sync":
				b.fault = func(stage string) error {
					if stage == "sync:open" {
						return io.ErrUnexpectedEOF
					}
					return nil
				}
			}
			if _, err := b.Record(context.Background(), s, testNow); !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatal("I/O not unknown", err)
			}
			b.Close()
			reopened := openBuffer(t, root, "", 0, 10)
			if r := getRecord(t, reopened, hash); r.Count != 2 {
				t.Fatal("I/O recovery count", r.Count)
			}
			if count, err := reopened.Count(context.Background(), "all"); err != nil || count != 1 {
				t.Fatal("I/O recovery duplicates", err)
			}
		})
	}
}

func TestBufferCorruptionAndPaths(t *testing.T) {
	for _, kind := range []string{"bad-yaml", "wrong-hash", "unknown-field", "journal", "overflow", "record-link", "bucket-link", "lock-link"} {
		t.Run(kind, func(t *testing.T) {
			root := privateTemp(t)
			b := openBuffer(t, root, "", 0, 10)
			s := testSnapshot()
			hash := mustRecord(t, b, s, testNow)
			recordPath := filepath.Join(b.path, "open", hash+".yaml")
			original, _ := os.ReadFile(recordPath)
			watched := recordPath
			switch kind {
			case "bad-yaml":
				os.WriteFile(recordPath, []byte("not: [valid"), 0600)
			case "wrong-hash":
				os.WriteFile(recordPath, bytes.Replace(original, []byte(hash), []byte("aaaaaaaaaaaa"), 1), 0600)
			case "unknown-field":
				os.WriteFile(recordPath, append(original, []byte("future_field: keep\n")...), 0600)
			case "journal":
				watched = filepath.Join(b.path, "intent.yaml")
				os.WriteFile(watched, []byte("invalid: ["), 0600)
			case "overflow":
				watched = filepath.Join(b.path, "overflow", hash+".yaml")
				os.WriteFile(watched, []byte("bad"), 0600)
			case "record-link":
				os.Remove(recordPath)
				os.Symlink(filepath.Join(root, "outside"), recordPath)
			case "bucket-link":
				os.Remove(recordPath)
				os.Remove(filepath.Join(b.path, "open"))
				os.Symlink(root, filepath.Join(b.path, "open"))
			case "lock-link":
				os.Remove(filepath.Join(b.path, ".buffer.lock"))
				os.Symlink(recordPath, filepath.Join(b.path, ".buffer.lock"))
			}
			before, _ := os.ReadFile(watched)
			if _, err := b.Record(context.Background(), s, testNow); err == nil {
				t.Fatal("corruption ignored")
			}
			after, _ := os.ReadFile(watched)
			if !bytes.Equal(before, after) {
				t.Fatal("corrupt file overwritten")
			}
		})
	}
	root := privateTemp(t)
	os.Chmod(root, 0755)
	if b, err := New(Config{Root: root}, testBuilder(t)); err == nil {
		b.Close()
		t.Fatal("public root accepted")
	}
}

func TestBufferYAMLRoundTrip(t *testing.T) {
	b := openBuffer(t, privateTemp(t), "", 0, 10)
	s := testSnapshot()
	s.Request.Method = "POST"
	s.Payload = mask.OrderedObject{{Key: "z", Value: int64(1)}, {Key: "a", Value: []any{"ordinary", true}}}
	s.Trace.Full = string([]byte{'x', 0xff, 0xfe})
	hash := mustRecord(t, b, s, testNow)
	r := getRecord(t, b, hash)
	if r.Trace.Full != s.Trace.Full {
		t.Fatal("binary trace bytes changed")
	}
	object, ok := r.Payload.(mask.OrderedObject)
	if !ok || len(object) != 2 || object[0].Key != "z" || object[1].Key != "a" || object[0].Value != int64(1) {
		t.Fatalf("ordered payload %#v", r.Payload)
	}
	s.Payload = make([]any, 5001)
	for i := range s.Payload.([]any) {
		s.Payload.([]any)[i] = int64(i)
	}
	mustRecord(t, b, s, testNow)
	object, ok = getRecord(t, b, hash).Payload.(mask.OrderedObject)
	if !ok || len(object) != 5001 || object[5000].Key != "__truncated__" {
		t.Fatal("array marker shape")
	}
	s.Payload = []any{}
	mustRecord(t, b, s, testNow)
	if value, ok := getRecord(t, b, hash).Payload.([]any); !ok || len(value) != 0 {
		t.Fatal("empty sequence")
	}
	data, _ := os.ReadFile(filepath.Join(b.path, "open", hash+".yaml"))
	if !bytes.Contains(data, []byte("!!binary")) {
		t.Fatal("binary YAML evidence absent")
	}
}

func TestBufferConcurrent(t *testing.T) {
	root := privateTemp(t)
	b := openBuffer(t, root, "", 0, 100)
	s := testSnapshot()
	const workers = 12
	errorsCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other, err := New(Config{Root: root, DailyCap: intPointer(0)}, testBuilder(t))
			if err != nil {
				errorsCh <- err
				return
			}
			defer other.Close()
			_, err = other.Record(context.Background(), s, testNow)
			errorsCh <- err
		}()
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if r := getRecord(t, b, b.builder.Hash(s.Exception)); r.Count != workers {
		t.Fatal("concurrent lost count", r.Count)
	}
	maxRoot := privateTemp(t)
	limited := openBuffer(t, maxRoot, "", 0, 3)
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			other, err := New(Config{Root: maxRoot, DailyCap: intPointer(0), MaxOpen: intPointer(3)}, testBuilder(t))
			if err != nil {
				results <- err
				return
			}
			defer other.Close()
			unique := s
			unique.Exception.Line = id
			_, err = other.Record(context.Background(), unique, testNow)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrFull) {
			t.Fatal(err)
		}
	}
	if n, err := limited.Count(context.Background(), "open"); err != nil || n != 3 || success != 3 {
		t.Fatal("capacity race", n, success, err)
	}
	lock, err := b.lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waiting := openBuffer(t, root, "", 0, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := waiting.Record(ctx, s, testNow); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lock deadline", err)
	}
	lock.Close()
}

func intPointer(value int) *int { return &value }

func TestBufferProcess(t *testing.T) {
	if root := os.Getenv("GO_MOO_BUFFER_HELPER_ROOT"); root != "" {
		b := openBuffer(t, root, "", 0, 100)
		for i := 0; i < 5; i++ {
			mustRecord(t, b, testSnapshot(), testNow)
		}
		return
	}
	root := privateTemp(t)
	b := openBuffer(t, root, "", 0, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	commands := []*exec.Cmd{}
	for i := 0; i < 2; i++ {
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBufferProcess$", "-test.count=1")
		command.Env = append(os.Environ(), "GO_MOO_BUFFER_HELPER_ROOT="+root)
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, command)
		t.Cleanup(func() {
			if command.ProcessState == nil {
				command.Process.Kill()
				command.Wait()
			}
		})
	}
	for _, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatal("buffer helper failed", err)
		}
	}
	if r := getRecord(t, b, b.builder.Hash(testSnapshot().Exception)); r.Count != 10 {
		t.Fatal("cross process lost count", r.Count)
	}
}

func privateTemp(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestBufferSourceFixture(t *testing.T) {
	data, err := os.ReadFile("../docs/contracts/fixtures/runtime-buffer.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		CapacityResult *string `json:"capacity_result"`
		Steps          []struct {
			Name, Hash string
			Buckets    []string
			Record     json.RawMessage
			CountAll   int `json:"count_all"`
			CountOpen  int `json:"count_open"`
			Rows       json.RawMessage
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Steps) != 5 || fixture.CapacityResult != nil {
		t.Fatal("source fixture shape")
	}
	var initial records.Record
	if err = json.Unmarshal(fixture.Steps[0].Record, &initial); err != nil {
		t.Fatal(err)
	}
	b := openBuffer(t, privateTemp(t), "", 2, 1)
	snapshot := testSnapshot()
	snapshot.Exception.Code = initial.Exception.Code
	snapshot.Context = records.GoContext{Env: "unknown", Project: "unknown", Go: "fixture-go", Goravel: "fixture-goravel"}
	snapshot.Trace = initial.Trace
	snapshot.Snippet = initial.Snippet
	compare := func(index int) {
		t.Helper()
		step := fixture.Steps[index]
		got := getRecord(t, b, step.Hash)
		var expected map[string]any
		if err := json.Unmarshal(step.Record, &expected); err != nil {
			t.Fatal(err)
		}
		ctxFields := expected["context"].(map[string]any)
		delete(ctxFields, "php")
		delete(ctxFields, "laravel")
		ctxFields["go"] = "fixture-go"
		ctxFields["goravel"] = "fixture-goravel"
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var actual any
		if err = json.Unmarshal(encoded, &actual); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, expected) {
			for key, value := range expected {
				if !reflect.DeepEqual(actual.(map[string]any)[key], value) {
					t.Log("different field", key)
				}
			}
			t.Fatalf("source record differs at %s", step.Name)
		}
		rows, err := b.List(context.Background(), "all", 100)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err = json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		var actualRows, expectedRows any
		json.Unmarshal(encoded, &actualRows)
		json.Unmarshal(step.Rows, &expectedRows)
		if !reflect.DeepEqual(actualRows, expectedRows) {
			t.Fatalf("source rows differ at %s", step.Name)
		}
		total, err := b.Count(context.Background(), "all")
		if err != nil || total != step.CountAll {
			t.Fatal("source count all")
		}
		total, err = b.Count(context.Background(), "open")
		if err != nil || total != step.CountOpen {
			t.Fatal("source count open")
		}
	}
	now, _ := time.Parse(time.RFC3339Nano, initial.FirstSeen)
	hash := mustRecord(t, b, snapshot, now)
	if hash != fixture.Steps[0].Hash {
		t.Fatal("source hash")
	}
	compare(0)
	for i := 2; i <= 5; i++ {
		mustRecord(t, b, snapshot, now)
	}
	compare(1)
	now, _ = time.Parse(time.RFC3339Nano, "2026-10-30T00:00:00.001+08:00")
	mustRecord(t, b, snapshot, now)
	compare(2)
	for index, bucket := range []string{"resolved", "deleted"} {
		seedBucket(t, b, hash, bucket)
		mustRecord(t, b, snapshot, now)
		compare(index + 3)
	}
}

func TestBufferCancellationAndCause(t *testing.T) {
	b := openBuffer(t, privateTemp(t), "", 0, 10)
	s := testSnapshot()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	b.fault = func(stage string) error {
		if stage == "intent-durable" {
			close(entered)
			<-release
		}
		return nil
	}
	go func() { _, err := b.Record(context.Background(), s, testNow); done <- err }()
	<-entered
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	second := make(chan error, 1)
	go func() { _, err := b.Get(ctx, b.builder.Hash(s.Exception)); second <- err }()
	select {
	case err := <-second:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("same-buffer gate deadline", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("same-buffer queue ignored deadline")
	}
}

func TestBufferFailureCause(t *testing.T) {
	b := openBuffer(t, privateTemp(t), "", 0, 10)
	s := testSnapshot()
	hash := mustRecord(t, b, s, testNow)
	injected := errors.New("injected-safe-io-cause")
	b.fault = func(stage string) error {
		if stage == "rename:open/"+hash+".yaml" {
			return injected
		}
		return nil
	}
	if _, err := b.Record(context.Background(), s, testNow); !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, injected) {
		t.Fatal("cause was lost", err)
	}
	if _, err := b.Get(context.Background(), hash); !errors.Is(err, injected) {
		t.Fatal("recovery cause lost", err)
	}
	b.fault = nil
	if r := getRecord(t, b, hash); r.Count != 2 {
		t.Fatal("error recovery count", r.Count)
	}
}

func TestBufferProcessInterruption(t *testing.T) {
	if root := os.Getenv("GO_MOO_BUFFER_CRASH_ROOT"); root != "" {
		b := openBuffer(t, root, "", 0, 10)
		b.fault = func(stage string) error {
			if stage == "target-durable" {
				os.Exit(17)
			}
			return nil
		}
		mustRecord(t, b, testSnapshot(), testNow.Add(time.Hour))
		t.Fatal("process did not exit at target stage")
	}
	root := privateTemp(t)
	b := openBuffer(t, root, "", 0, 10)
	s := testSnapshot()
	hash := mustRecord(t, b, s, testNow)
	seedBucket(t, b, hash, "deleted")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBufferProcessInterruption$", "-test.count=1")
	child.Env = append(os.Environ(), "GO_MOO_BUFFER_CRASH_ROOT="+root)
	child.Stdout = io.Discard
	child.Stderr = io.Discard
	err := child.Run()
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() != 17 {
		t.Fatal("expected abrupt process exit17", err)
	}
	// This reads the deliberately interrupted disk state before opening a recovery operation.
	for _, bucket := range []string{"open", "deleted"} {
		if _, err := os.Stat(filepath.Join(b.path, bucket, hash+".yaml")); err != nil {
			t.Fatal("expected target+old before recovery", err)
		}
	}
	if _, err := os.Stat(filepath.Join(b.path, "intent.yaml")); err != nil {
		t.Fatal("missing durable intent", err)
	}
	b.Close()
	reopened := openBuffer(t, root, "", 0, 10)
	if r := getRecord(t, reopened, hash); r.Count != 2 || r.Status != "open" {
		t.Fatal("process recovery final image")
	}
	if count, err := reopened.Count(context.Background(), "all"); err != nil || count != 1 {
		t.Fatal("process recovery duplicate", err)
	}
	if getRecord(t, reopened, hash).Count != 2 {
		t.Fatal("process recovery double counted")
	}
}

func TestBufferDirectoryInitialization(t *testing.T) {
	root := privateTemp(t)
	b := openBuffer(t, root, "explicit", 0, 10)
	for _, dir := range []string{".", "open", "resolved", "deleted", "overflow"} {
		info, err := b.root.Stat(dir)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
			t.Fatal("private initialized directory", dir, err)
		}
	}
	count, err := b.Count(context.Background(), "all")
	if err != nil || count != 0 {
		t.Fatal("fresh buffer", count, err)
	}
}

func TestBufferIncompleteIntent(t *testing.T) {
	for _, omitted := range []string{"version", "hash", "target", "record", "overflow"} {
		t.Run(omitted, func(t *testing.T) {
			b := openBuffer(t, privateTemp(t), "", 1, 10)
			s := testSnapshot()
			hash := mustRecord(t, b, s, testNow)
			if err := b.writeOverflow(hash, 3); err != nil {
				t.Fatal(err)
			}
			if err := b.writeIntent(intent{Version: 1, Hash: hash, Target: "open", Overflow: 4}); err != nil {
				t.Fatal(err)
			}
			data, _, err := b.read("intent.yaml")
			if err != nil {
				t.Fatal(err)
			}
			lines := bytes.Split(data, []byte("\n"))
			kept := [][]byte{}
			for _, line := range lines {
				if !bytes.HasPrefix(line, []byte(omitted+":")) {
					kept = append(kept, line)
				}
			}
			corrupt := bytes.Join(kept, []byte("\n"))
			if err := b.root.WriteFile("intent.yaml", corrupt, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := b.Get(context.Background(), hash); err == nil {
				t.Fatal("incomplete intent accepted")
			}
			current, _, _ := b.read("intent.yaml")
			if !bytes.Equal(current, corrupt) {
				t.Fatal("incomplete intent overwritten")
			}
			if value, err := b.readOverflow(hash); err != nil || value != 3 {
				t.Fatal("incomplete intent changed overflow", value, err)
			}
		})
	}
}

func TestBufferIntentTypes(t *testing.T) {
	for _, field := range []string{"overflow", "version", "record"} {
		t.Run(field, func(t *testing.T) {
			b := openBuffer(t, privateTemp(t), "", 1, 10)
			s := testSnapshot()
			hash := mustRecord(t, b, s, testNow)
			record := getRecord(t, b, hash)
			original, _, _ := b.read("open/" + hash + ".yaml")
			if err := b.writeOverflow(hash, 3); err != nil {
				t.Fatal(err)
			}
			if err := b.writeIntent(intent{Version: 1, Hash: hash, Target: "open", Record: record, Overflow: 0}); err != nil {
				t.Fatal(err)
			}
			data, _, err := b.read("intent.yaml")
			if err != nil {
				t.Fatal(err)
			}
			var doc yaml.Node
			if err = yaml.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < len(doc.Content[0].Content); i += 2 {
				if doc.Content[0].Content[i].Value == field {
					bad := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
					if field == "record" {
						bad = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{}}
					}
					doc.Content[0].Content[i+1] = bad
				}
			}
			corrupt, err := yaml.Marshal(&doc)
			if err != nil {
				t.Fatal(err)
			}
			if err = b.root.WriteFile("intent.yaml", corrupt, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = b.Get(context.Background(), hash); err == nil {
				t.Fatal("invalid intent node accepted")
			}
			current, _, _ := b.read("intent.yaml")
			if !bytes.Equal(current, corrupt) {
				t.Fatal("invalid intent overwritten")
			}
			current, _, _ = b.read("open/" + hash + ".yaml")
			if !bytes.Equal(current, original) {
				t.Fatal("invalid intent changed record")
			}
			if value, err := b.readOverflow(hash); err != nil || value != 3 {
				t.Fatal("invalid intent changed overflow", value, err)
			}
		})
	}
}
