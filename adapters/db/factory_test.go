package dbcapture

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/slowquery"
	frameworkconfig "github.com/goravel/framework/config"
	contractsdb "github.com/goravel/framework/contracts/database/db"
	contractsdriver "github.com/goravel/framework/contracts/database/driver"
	"github.com/goravel/framework/contracts/log"
	native "github.com/goravel/framework/database/db"
	nativedriver "github.com/goravel/framework/database/driver"
	"github.com/goravel/sqlite"
)

type memoryLog struct {
	log.Log
	traces atomic.Int64
}

func (m *memoryLog) WithContext(context.Context) log.Log { return m }
func (m *memoryLog) Infof(string, ...any)                { m.traces.Add(1) }
func (m *memoryLog) Warningf(string, ...any)             { m.traces.Add(1) }
func (m *memoryLog) Errorf(string, ...any)               { m.traces.Add(1) }
func (m *memoryLog) Panicf(string, ...any)               { panic("original database failure") }

type actorKey struct{}

func TestFactorySamePoolContextAndTransactions(t *testing.T) {
	root := t.TempDir()
	os.Chmod(root, 0700)
	envFile := filepath.Join(root, "empty.env")
	os.WriteFile(envFile, nil, 0600)
	cfg := frameworkconfig.NewApplication(envFile)
	cfg.Add("app", map[string]any{"debug": true})
	sink := &memoryLog{}
	var first, second atomic.Int64
	cfg.Add("database", map[string]any{"default": "db32-first", "slow_threshold": 200, "connections": map[string]any{
		"db32-first": map[string]any{"database": filepath.Join(root, "first.sqlite"), "via": func() (contractsdriver.Driver, error) {
			first.Add(1)
			return sqlite.NewSqlite(cfg, sink, "db32-first"), nil
		}},
		"db32-second": map[string]any{"database": filepath.Join(root, "second.sqlite"), "via": func() (contractsdriver.Driver, error) {
			second.Add(1)
			return sqlite.NewSqlite(cfg, sink, "db32-second"), nil
		}},
	}})
	t.Cleanup(nativedriver.CloseConnections)
	// Establish the business pool using the original public path before monitoring.
	original := sqlite.NewSqlite(cfg, sink, "db32-first")
	logger := native.NewLogger(cfg, sink)
	gm, instrument, err := nativedriver.BuildGorm(cfg, logger.ToGorm(), original.Pool(), "db32-first", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = gm.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, token TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	originalSQL, err := gm.DB()
	if err != nil {
		t.Fatal(err)
	}
	threshold := float64(0)
	builder, _ := slowquery.New(slowquery.Config{Enabled: true, ThresholdMillis: &threshold, MaskKeys: []string{"token"}})
	cap := 0
	store, err := buffer.NewSlow(buffer.Config{Root: root, Scope: "factory", DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var tick atomic.Int64
	actor := context.WithValue(context.Background(), actorKey{}, "actor-a")
	db, owner, err := New(actor, cfg, sink, nil, "db32-first", Config{Enabled: true, Recorder: store, Clock: func() time.Time { return time.Now().Add(time.Duration(tick.Add(1)) * time.Millisecond) }, Callsite: func() (string, int) { return "app/db-factory.go", 1 }, Request: func(ctx context.Context) (slowquery.Request, error) {
		id, _ := ctx.Value(actorKey{}).(string)
		return slowquery.Request{Method: "POST", UserID: &id}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	observed := db.(*observedDB)
	sqlPool, err := observed.state.gorm.DB()
	if err != nil || sqlPool != originalSQL || observed.state.instrument != instrument || first.Load() != 1 {
		t.Fatal("parallel pool or repeated driver", err, first.Load())
	}
	beforeTrace := sink.traces.Load()
	if _, err = db.Table("items").Insert(map[string]any{"id": int64(1), "token": "fixed-insert-secret"}); err != nil {
		t.Fatal(err)
	}
	var values []struct {
		ID    int64
		Token string
	}
	if err = db.Select(&values, "SELECT id,token FROM items WHERE id=?", int64(1)); err != nil || len(values) != 1 {
		t.Fatal("original direct select", err)
	}
	if owner.Status().Captured != 2 || sink.traces.Load() != beforeTrace+2 {
		t.Fatal("original logger/one observation")
	}
	ctxB := context.WithValue(context.Background(), actorKey{}, "actor-b")
	current := db.WithContext(ctxB).Connection("db32-first")
	if first.Load() != 1 {
		t.Fatal("context opened a pool")
	}
	tx, err := current.BeginTransaction()
	if err != nil {
		t.Fatal(err)
	}
	beforeTrace = sink.traces.Load()
	if _, err = tx.Table("items").Where("id", 1).Update("token", "fixed-commit-secret"); err != nil {
		t.Fatal(err)
	}
	if sink.traces.Load() != beforeTrace {
		t.Fatal("transaction table trace logged before commit")
	}
	if err = tx.Statement("UPDATE items SET token=? WHERE id=?", "fixed-direct-secret", int64(1)); err != nil {
		t.Fatal(err)
	}
	if owner.Status().Captured != 4 {
		t.Fatal("transaction double counted", owner.Status())
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if sink.traces.Load() != beforeTrace+2 {
		t.Fatal("original transaction trace timing lost")
	}
	rollback := errors.New("business rollback")
	// Use the real contract callback and check that a rolled-back successful SQL remains observed.
	err = current.Transaction(func(tx contractsdb.Tx) error {
		if _, err := tx.Table("items").Where("id", 1).Update("token", "fixed-rollback-secret"); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("transaction error replaced", err)
	}
	if err = current.Select(&values, "SELECT id,token FROM items WHERE id=?", int64(1)); err != nil || values[0].Token != "fixed-direct-secret" {
		t.Fatal("rollback business result", err)
	}
	other := current.Connection("db32-second")
	if err = other.Statement("CREATE TABLE items (id INTEGER PRIMARY KEY, token TEXT)"); err != nil {
		t.Fatal(err)
	}
	if second.Load() != 1 {
		t.Fatal("second driver not single construct")
	}
	if other != current.Connection("db32-second") {
		t.Fatal("connection wrapper cache lost")
	}
	snapshots, err := store.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seenB, seenOther := false, false
	for _, snapshot := range snapshots {
		r := snapshot.Record
		if r.Request.UserID != nil && *r.Request.UserID == "actor-b" {
			seenB = true
		}
		if r.At.Connection != nil && *r.At.Connection == "db32-second" {
			seenOther = true
		}
	}
	if !seenB || !seenOther {
		t.Fatal("context/connection lost")
	}
	captured := owner.Status().Captured
	if err = owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = current.Statement("UPDATE items SET token=? WHERE id=?", "after close", int64(1)); err != nil || owner.Status().Captured != captured {
		t.Fatal("close changed business or captured", err)
	}
}

func TestFactoryDriverFailureAndDisabledCapture(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty.env")
	os.WriteFile(empty, nil, 0600)
	cfg := frameworkconfig.NewApplication(empty)
	sink := &memoryLog{}
	original := errors.New("original-driver-failure")
	var calls atomic.Int64
	cfg.Add("database", map[string]any{"default": "db32-disabled", "connections": map[string]any{
		"db32-failure":  map[string]any{"via": func() (contractsdriver.Driver, error) { calls.Add(1); return nil, original }},
		"db32-disabled": map[string]any{"database": filepath.Join(root, "disabled.sqlite"), "via": func() (contractsdriver.Driver, error) { return sqlite.NewSqlite(cfg, sink, "db32-disabled"), nil }},
	}})
	t.Cleanup(nativedriver.CloseConnections)
	if _, _, err := New(context.Background(), cfg, sink, nil, "db32-failure", Config{}); !errors.Is(err, original) || calls.Load() != 1 {
		t.Fatal("driver error changed", err, calls.Load())
	}
	db, owner, err := New(context.Background(), cfg, sink, nil, "db32-disabled", Config{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if owner.Status().Ready || owner.Status().Last.Kind != "configuration" {
		t.Fatal(owner.Status())
	}
	if _, wrapped := db.(*observedDB); wrapped {
		t.Fatal("invalid monitor wrapped business DB")
	}
	if err = db.Statement("CREATE TABLE items (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err = db.Statement("INSERT INTO items(id) VALUES (?)", 1); err != nil {
		t.Fatal(err)
	}
	var rows []struct{ ID int64 }
	if err = db.Select(&rows, "SELECT id FROM items"); err != nil || len(rows) != 1 {
		t.Fatal(err, len(rows))
	}
	if err = owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
