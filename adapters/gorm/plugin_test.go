package gormcapture_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	observer "gitee.com/charsen/moo-go-monitor/adapters/gorm"
	"gitee.com/charsen/moo-go-monitor/buffer"
	"gitee.com/charsen/moo-go-monitor/slowquery"
	"github.com/goravel/framework/contracts/database"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	frameworkgorm "github.com/goravel/framework/database/gorm"
	"github.com/goravel/framework/foundation"
	"github.com/goravel/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestMain(m *testing.M) {
	// Register only the actual driver provider; no Host bootstrap or Boot.
	(&sqlite.ServiceProvider{}).Register(foundation.NewApplication())
	os.Exit(m.Run())
}

type nativeTrace struct {
	mu    sync.Mutex
	calls int
}

func (n *nativeTrace) LogMode(gormlogger.LogLevel) gormlogger.Interface { return n }
func (*nativeTrace) Info(context.Context, string, ...any)               {}
func (*nativeTrace) Warn(context.Context, string, ...any)               {}
func (*nativeTrace) Error(context.Context, string, ...any)              {}
func (n *nativeTrace) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	fc()
	n.mu.Lock()
	n.calls++
	n.mu.Unlock()
}
func (n *nativeTrace) count() int { n.mu.Lock(); defer n.mu.Unlock(); return n.calls }

type item struct {
	ID   int64
	Name string
}

func (item) TableName() string { return "items" }
func setup(t *testing.T, label string, threshold float64, skip ...string) (*buffer.SlowBuffer, *frameworkgorm.Query, *gorm.DB, *nativeTrace, func() time.Time) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	native := &nativeTrace{}
	db, err := gorm.Open(sqlite.Open(filepath.Join(root, "fixture.sqlite")), &gorm.Config{Logger: native, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)
	if err = db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	builder, err := slowquery.New(slowquery.Config{Enabled: true, ThresholdMillis: &threshold, SkipPatterns: skip, MaskKeys: []string{"token"}, Env: "testing", Project: "gorm-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	cap := 0
	store, err := buffer.NewSlow(buffer.Config{Root: root, Scope: label, DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	query := frameworkgorm.NewQuery(context.Background(), nil, database.Config{Connection: label, Driver: "sqlite"}, db, sqlite.NewGrammar(nil, ""), nil, nil, nil, nil)
	var ticks atomic.Int64
	clock := func() time.Time {
		return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC).Add(time.Duration(ticks.Add(1)) * 100 * time.Millisecond)
	}
	return store, query, db, native, clock
}
func plugin(t *testing.T, store observer.Recorder, label string, clock func() time.Time) *observer.Plugin {
	t.Helper()
	p, err := observer.New(observer.Config{Connection: label, Recorder: store, Clock: clock, CaptureTimeout: 2 * time.Second, Callsite: func() (string, int) { return "app/query.go", 12 }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close(context.Background()) })
	return p
}
func TestGoravelORMCallbacks(t *testing.T) {
	store, q, db, native, clock := setup(t, "one", 0)
	p := plugin(t, store, "one", clock)
	if err := observer.Install(q, p); err != nil {
		t.Fatal(err)
	}
	if err := observer.Install(q, p); err != nil {
		t.Fatal("same owner not idempotent")
	}
	other := plugin(t, store, "one", clock)
	if err := observer.Install(q, other); !errors.Is(err, observer.ErrInstalled) {
		t.Fatal("other owner accepted")
	}
	count := native.count()
	value := item{Name: "fixture"}
	if err := q.Create(&value); err != nil || value.ID == 0 {
		t.Fatal("ORM create failed", err)
	}
	var values []item
	if err := q.Where("id", value.ID).Get(&values); err != nil || len(values) != 1 {
		t.Fatal("ORM query failed", err)
	}
	if _, err := q.Model(&item{}).Where("id", value.ID).Update("name", "updated"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Exec("UPDATE items SET name=? WHERE id=?", "raw-value", value.ID); err != nil {
		t.Fatal(err)
	}
	var total int64
	if err := db.Raw("SELECT COUNT(*) FROM items").Row().Scan(&total); err != nil || total != 1 {
		t.Fatal("row result changed", err)
	}
	if _, err := q.Where("id", value.ID).Delete(&item{}); err != nil {
		t.Fatal(err)
	}
	if native.count()-count != 6 {
		t.Fatal("native Trace not called exactly once per operation", native.count()-count)
	}
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 6 || p.Status().Captured != 6 || p.Status().Failures != 0 {
		t.Fatal("six callbacks missing", err, p.Status())
	}
	for _, s := range snapshots {
		if s.Record.At.Connection == nil || *s.Record.At.Connection != "one" {
			t.Fatal("connection label missing")
		}
	}
	ctx := context.WithValue(context.Background(), struct{}{}, "context-marker")
	if err := q.WithContext(ctx).Create(&item{Name: "context"}); err != nil {
		t.Fatal(err)
	}
	tx, err := q.BeginTransaction()
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Create(&item{Name: "rollback"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = db.Raw("SELECT COUNT(*) FROM items WHERE name='rollback'").Row().Scan(&total); err != nil || total != 0 {
		t.Fatal("transaction result changed", err)
	}
	if p.Status().Captured != 9 {
		t.Fatal("rollback incorrectly erased successful SQL samples", p.Status())
	}
	if err = p.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := p.Status().Captured
	if err = q.Create(&item{Name: "after close"}); err != nil {
		t.Fatal("closed plugin changed SQL", err)
	}
	if p.Status().Captured != before {
		t.Fatal("closed plugin captured")
	}
	if err = observer.Install(q, p); !errors.Is(err, observer.ErrClosed) {
		t.Fatal("closed owner silently reinstalled")
	}
}

type hookItem struct {
	ID     int64
	Name   string
	Nested int64 `gorm:"-"`
}

func (hookItem) TableName() string { return "items" }
func (row *hookItem) AfterFind(tx *gorm.DB) error {
	row.Name = "after-hook"
	return tx.Session(&gorm.Session{NewDB: true}).Table("items").Count(&row.Nested).Error
}
func TestORMCallbackWindowAndErrors(t *testing.T) {
	store, q, db, native, clock := setup(t, "window", 0)
	if err := q.Create(&item{Name: "original"}); err != nil {
		t.Fatal(err)
	}
	p := plugin(t, store, "window", clock)
	if err := observer.Install(q, p); err != nil {
		t.Fatal(err)
	}
	before := native.count()
	var values []hookItem
	if err := q.Get(&values); err != nil || len(values) != 1 || values[0].Name != "after-hook" || values[0].Nested != 1 {
		t.Fatal("after hook behavior changed", err)
	}
	if native.count()-before != 2 {
		t.Fatal("nested query/native Trace changed")
	}
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 2 {
		t.Fatal("main/nested SQL overwritten", err, p.Status())
	}
	foundSelect, foundCount := false, false
	for _, s := range snapshots {
		if strings.HasPrefix(s.Record.SQL.Raw, "SELECT *") {
			foundSelect = true
		}
		if strings.Contains(strings.ToLower(s.Record.SQL.Raw), "count(") {
			foundCount = true
		}
	}
	if !foundSelect || !foundCount {
		t.Fatal("wrong SQL cleanup window")
	}
	prior := p.Status().Captured
	if err = db.Session(&gorm.Session{DryRun: true}).Find(&[]item{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = q.Exec("SELECT * FROM nonexistent_monitor_table"); err == nil {
		t.Fatal("SQL error swallowed")
	}
	if p.Status().Captured != prior {
		t.Fatal("DryRun/SQL execution failure captured")
	}
	var absent item
	if err = db.Where("id = ?", 999999).First(&absent).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("record-not-found changed", err)
	}
	if p.Status().Captured != prior+1 {
		t.Fatal("successful empty SELECT skipped")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = q.WithContext(ctx).Exec("SELECT 1"); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled SQL error changed", err)
	}
	if p.Status().Captured != prior+1 {
		t.Fatal("canceled execution captured")
	}
}

var _ contractsorm.Query = (*frameworkgorm.Query)(nil)

func TestInstallationRollback(t *testing.T) {
	store, q, db, _, clock := setup(t, "install", 0)
	p := plugin(t, store, "install", clock)
	// GORM evaluates public Match callbacks while compiling each new registration.
	// Fail once only after several of this owner's callbacks have been appended.
	var failed bool
	if err := db.Callback().Query().Match(func(tx *gorm.DB) bool {
		if !failed && tx.Callback().Query().Get("moo-monitor:slow-query:after_query") != nil {
			failed = true
			panic("fixed registration failure")
		}
		return true
	}).Register("business:existing", func(*gorm.DB) {}); err != nil {
		t.Fatal(err)
	}
	if err := observer.Install(q, p); !errors.Is(err, observer.ErrConfiguration) {
		t.Fatal("installation failure missing", err)
	}
	if !failed || db.Callback().Query().Get("business:existing") == nil || db.Callback().Query().Get("gorm:query") == nil {
		t.Fatal("original callbacks lost")
	}
	if db.Callback().Create().Get("moo-monitor:slow-query:before_create") != nil || db.Callback().Query().Get("moo-monitor:slow-query:after_query") != nil {
		t.Fatal("partial callbacks retained")
	}
	var result int64
	if err := db.Raw("SELECT 1").Row().Scan(&result); err != nil || result != 1 {
		t.Fatal("rollback broke SQL", err)
	}
	if err := observer.Install(q, p); err != nil {
		t.Fatal("clean reinstall failed", err)
	}
	if err := q.Create(&item{Name: "after reinstall"}); err != nil || p.Status().Captured != 1 {
		t.Fatal("reinstall did not capture", err, p.Status())
	}
}
