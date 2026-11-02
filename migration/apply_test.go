package migration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitee.com/charsen/moo-go-monitor/buffer"
	"gitee.com/charsen/moo-go-monitor/cloudsync"
	"gitee.com/charsen/moo-go-monitor/records"
	"gitee.com/charsen/moo-go-monitor/slowquery"
	"go.yaml.in/yaml/v3"
)

func importFixture(t *testing.T) (*Plan, string, string) {
	t.Helper()
	base := t.TempDir()
	runtime := filepath.Join(base, "legacy-runtime")
	slow := filepath.Join(base, "legacy-slow")
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	for _, source := range []struct {
		kind, root, bucket, hash string
		count                    int
	}{{"runtimes", runtime, "open", "3e4258636dd8", 5}, {"slow_sql", slow, "resolved", "38a113eb9039", 7}} {
		var value map[string]any
		if err := yaml.Unmarshal(fakeRecord(t, source.kind), &value); err != nil {
			t.Fatal(err)
		}
		value["count"] = source.count
		value["status"] = source.bucket
		if source.bucket == "resolved" {
			value["resolved_at"] = value["last_seen"]
		}
		data, err := yaml.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		place(t, source.root, source.bucket, source.hash+".yaml", data)
	}
	plan, err := BuildPlan(context.Background(), Sources{RuntimeSource: runtime, SlowSource: slow}, Options{TargetRoot: target, Scope: "migration-fixture", MaskKeys: []string{"password", "token"}})
	if err != nil {
		t.Fatal(err)
	}
	return plan, runtime, target
}

type importSender struct{ t *testing.T }

func (s importSender) Send(ctx context.Context, batch []buffer.Snapshot) (cloudsync.Receipt, error) {
	if len(batch) != 1 || batch[0].Record.Count != 5 {
		s.t.Fatal("imported runtime not pending or count reset")
	}
	return cloudsync.Receipt{Saved: 1, Results: []cloudsync.Item{{Index: 0, Hash: batch[0].Hash, Status: "saved"}}}, nil
}
func (s importSender) SendSlow(ctx context.Context, batch []buffer.SlowSnapshot) (cloudsync.Receipt, error) {
	if len(batch) != 1 || batch[0].Bucket != "resolved" || batch[0].Record.Count != 7 {
		s.t.Fatal("imported slow resolved not pending or count reset")
	}
	return cloudsync.Receipt{Saved: 1, Results: []cloudsync.Item{{Index: 0, Hash: batch[0].Hash, Status: "saved"}}}, nil
}
func TestApplyImportAndPendingSync(t *testing.T) {
	plan, source, target := importFixture(t)
	before, err := os.ReadFile(filepath.Join(source, "open", "3e4258636dd8.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(context.Background())
	if err != nil || !result.Complete || result.Created != 2 {
		t.Fatal(result, err)
	}
	result, err = plan.Apply(context.Background())
	if err != nil || result.Created != 0 || result.AlreadyPresent != 2 || !result.Complete {
		t.Fatal("not idempotent", result, err)
	}
	runtimeBuilder, err := records.New(records.Config{})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := buffer.New(buffer.Config{Root: target, Scope: "migration-fixture"}, runtimeBuilder)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	slowBuilder, err := slowquery.New(slowquery.Config{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	slow, err := buffer.NewSlow(buffer.Config{Root: target, Scope: "migration-fixture"}, slowBuilder)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	reader, err := cloudsync.NewStatusReader(runtime, slow)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := reader.Stats(context.Background(), cloudsync.PushBoth)
	reader.Close()
	if err != nil || stats.Runtimes.Counts.PendingOpen != 1 || stats.SlowSQL.Counts.PendingResolved != 1 {
		t.Fatal("import invented ACK", stats, err)
	}
	push, err := cloudsync.NewPusher(runtime, slow, importSender{t}, cloudsync.PushConfig{Runtimes: true, SlowSQL: true, Timeout: 1000000000})
	if err != nil {
		t.Fatal(err)
	}
	pushed, err := push.Push(context.Background(), cloudsync.PushBoth)
	push.Close(context.Background())
	if err != nil || pushed.Runtimes.Confirmed != 1 || pushed.SlowSQL.Confirmed != 1 {
		t.Fatal(pushed, err)
	}
	after, err := os.ReadFile(filepath.Join(source, "open", "3e4258636dd8.yaml"))
	if err != nil || string(after) != string(before) {
		t.Fatal("source was moved or altered")
	}
	if _, err = plan.Apply(context.Background()); err == nil {
		t.Fatal("active ACK scope accepted for reimport")
	}
}
func TestApplyImportResumeAndOwnership(t *testing.T) {
	for _, mode := range []string{"partial", "unowned", "foreign-state", "journal", "cross-bucket", "manifest", "source-added", "source-deleted", "source-during", "tampered-plan"} {
		t.Run(mode, func(t *testing.T) {
			plan, source, target := importFixture(t)
			ctx := context.Background()
			sentinel := errors.New("fixed import interruption")
			if mode == "unowned" {
				dir := filepath.Join(target, "go-runtime-buffer--migration-fixture", "open")
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, plan.records[0].Hash+".yaml"), plan.records[0].Data, 0600); err != nil {
					t.Fatal(err)
				}
			} else if mode == "source-added" {
				place(t, source, "open", "unknown.yaml", []byte("must not silently omit"))
			} else if mode == "source-deleted" {
				if err := os.Remove(filepath.Join(source, "open", "3e4258636dd8.yaml")); err != nil {
					t.Fatal(err)
				}
			} else if mode == "tampered-plan" {
				plan.records[0].Data = append(plan.records[0].Data, 'x')
			} else {
				plan.fault = func(stage string, index int) error {
					if stage == "before-record" && index == 1 {
						return sentinel
					}
					return nil
				}
				first, err := plan.Apply(ctx)
				if !errors.Is(err, sentinel) || first.Created != 1 {
					t.Fatal("partial checkpoint", first, err)
				}
				plan.fault = nil
				scoped := filepath.Join(target, "go-runtime-buffer--migration-fixture")
				switch mode {
				case "foreign-state":
					if err := os.WriteFile(filepath.Join(scoped, "sync-runtimes.json"), []byte(`{}`), 0600); err != nil {
						t.Fatal(err)
					}
				case "cross-bucket":
					if err := os.Rename(filepath.Join(scoped, "open", "3e4258636dd8.yaml"), filepath.Join(scoped, "deleted", "3e4258636dd8.yaml")); err != nil {
						t.Fatal(err)
					}
				case "manifest":
					if err := os.WriteFile(filepath.Join(target, ".migration-migration-fixture.json"), []byte(`{"version":1}`), 0600); err != nil {
						t.Fatal(err)
					}
				case "journal":
					builder, _ := slowquery.New(slowquery.Config{Enabled: true})
					store, err := buffer.NewSlow(buffer.Config{Root: target, Scope: "migration-fixture"}, builder)
					if err != nil {
						t.Fatal(err)
					}
					store.Close()
					entry := plan.records[1]
					data, err := json.Marshal(buffer.ImportJournal{Version: 1, Owner: plan.Digest, Hash: entry.Hash, Bucket: entry.Bucket, Digest: entry.TargetDigest})
					if err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(target, "go-slow-query-buffer--migration-fixture", buffer.ImportJournalName), data, 0600); err != nil {
						t.Fatal(err)
					}
				case "source-during":
					plan.fault = func(stage string, index int) error {
						if stage == "before-record" && index == 1 {
							place(t, source, "open", "unknown.yaml", []byte("new source item"))
						}
						return nil
					}
				}
			}
			result, err := plan.Apply(ctx)
			if mode == "partial" || mode == "journal" {
				if err != nil || !result.Complete || result.AlreadyPresent != 1 || result.Created != 1 {
					t.Fatal("same sealed plan did not resume", result, err)
				}
			} else {
				if err == nil || result.Complete {
					t.Fatal("conflict accepted", result, err)
				}
			}
			if strings.HasPrefix(mode, "source-") && mode != "source-during" {
				entries, err := os.ReadDir(target)
				if err != nil || len(entries) != 0 {
					t.Fatal("source drift touched target", err)
				}
			}
		})
	}
}
