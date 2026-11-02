package gormcapture_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	observer "gitee.com/charsen/moo-go-monitor/adapters/gorm"
	"gitee.com/charsen/moo-go-monitor/buffer"
	"gitee.com/charsen/moo-go-monitor/slowquery"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/goravel/framework/contracts/database"
	frameworkgorm "github.com/goravel/framework/database/gorm"
	postgresdialect "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type pgRequestKey struct{}

type pgItem struct {
	ID    int64
	Token string
}

func (pgItem) TableName() string { return "items" }

type pgTrace struct{ calls atomic.Int64 }

func (p *pgTrace) LogMode(gormlogger.LogLevel) gormlogger.Interface { return p }
func (*pgTrace) Info(context.Context, string, ...any)               {}
func (*pgTrace) Warn(context.Context, string, ...any)               {}
func (*pgTrace) Error(context.Context, string, ...any)              {}

// Count native delivery without choosing to format values in the application's logger.
func (p *pgTrace) Trace(context.Context, time.Time, func() (string, int64), error) { p.calls.Add(1) }

type pgRecorder struct {
	store  *buffer.SlowBuffer
	mu     sync.Mutex
	inputs []slowquery.Input
}

func (p *pgRecorder) ReportSlowQuery(ctx context.Context, input slowquery.Input, now time.Time) (string, bool, error) {
	p.mu.Lock()
	p.inputs = append(p.inputs, input)
	p.mu.Unlock()
	return p.store.ReportSlowQuery(ctx, input, now)
}
func setupPostgres(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *buffer.SlowBuffer, *observer.Plugin, *pgRecorder, *pgTrace) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	native := &pgTrace{}
	db, err := gorm.Open(postgresdialect.New(postgresdialect.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{SkipDefaultTransaction: true, DisableAutomaticPing: true, Logger: native})
	if err != nil {
		t.Fatal(err)
	}
	db = db.WithContext(context.WithValue(context.Background(), pgRequestKey{}, "request-actor"))
	root := t.TempDir()
	os.Chmod(root, 0700)
	threshold := float64(0)
	builder, err := slowquery.New(slowquery.Config{Enabled: true, ThresholdMillis: &threshold, MaskKeys: []string{"token"}, Env: "testing", Project: "pg-mock"})
	if err != nil {
		t.Fatal(err)
	}
	cap := 0
	store, err := buffer.NewSlow(buffer.Config{Root: root, Scope: "pg", DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	recorder := &pgRecorder{store: store}
	var ticks atomic.Int64
	plugin, err := observer.New(observer.Config{Connection: "mock-postgres", Recorder: recorder, Clock: func() time.Time {
		return time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC).Add(time.Duration(ticks.Add(1)) * time.Millisecond)
	}, Callsite: func() (string, int) { return "app/mock-pg.go", 31 }, Request: func(ctx context.Context) (slowquery.Request, error) {
		id, ok := ctx.Value(pgRequestKey{}).(string)
		if !ok {
			return slowquery.Request{Method: "CLI"}, nil
		}
		return slowquery.Request{Method: "POST", UserID: &id}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { plugin.Close(context.Background()) })
	query := frameworkgorm.NewQuery(context.Background(), nil, database.Config{Connection: "mock-postgres", Driver: "postgres"}, db, nil, nil, nil, nil, nil)
	if err = observer.Install(query, plugin); err != nil {
		t.Fatal(err)
	}
	return db, mock, store, plugin, recorder, native
}
func TestPostgresCallbacks(t *testing.T) {
	db, mock, store, plugin, recorder, native := setupPostgres(t)
	mock.ExpectQuery(`INSERT INTO "items"`).WithArgs("fixed-create-secret").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(17))
	item := pgItem{Token: "fixed-create-secret"}
	if err := db.Create(&item).Error; err != nil || item.ID != 17 {
		t.Fatal("create callback", err)
	}
	mock.ExpectQuery(`SELECT \* FROM "items" WHERE id = \$1`).WithArgs(int64(17)).WillReturnRows(sqlmock.NewRows([]string{"id", "token"}).AddRow(17, "fixed-create-secret"))
	var rows []pgItem
	if err := db.WithContext(context.WithValue(context.Background(), pgRequestKey{}, "request-actor")).Where("id = ?", int64(17)).Find(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatal("query callback", err)
	}
	mock.ExpectExec(`UPDATE "items" SET "token"=\$1 WHERE id = \$2`).WithArgs("fixed-update-secret", int64(17)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := db.Model(&pgItem{}).Where("id = ?", int64(17)).Update("token", "fixed-update-secret").Error; err != nil {
		t.Fatal("update callback", err)
	}
	mock.ExpectExec(`DELETE FROM "items" WHERE id = \$1`).WithArgs(int64(17)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := db.Where("id = ?", int64(17)).Delete(&pgItem{}).Error; err != nil {
		t.Fatal("delete callback", err)
	}
	mock.ExpectQuery(`SELECT \$1 AS n`).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(42))
	var number int64
	if err := db.Raw("SELECT ? AS n", int64(42)).Row().Scan(&number); err != nil || number != 42 {
		t.Fatal("row callback", err)
	}
	mock.ExpectExec(`UPDATE items SET token=\$1 WHERE id=\$2`).WithArgs("fixed-exec-secret", int64(17)).WillReturnResult(sqlmock.NewResult(0, 1))
	result := db.Exec("UPDATE items SET token=? WHERE id=?", "fixed-exec-secret", int64(17))
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatal("raw callback", result.Error)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 6 || native.calls.Load() != 6 || plugin.Status().Captured != 6 {
		t.Fatal("six real GORM execution callbacks", len(snapshots), native.calls.Load(), plugin.Status(), err)
	}
	for _, input := range recorder.inputs {
		if input.RenderedSQL == nil || len(input.Bindings) != 0 || !strings.Contains(input.Raw, "$") {
			t.Fatal("raw/display boundary")
		}
	}
	for _, snapshot := range snapshots {
		record := snapshot.Record
		if record.At.Connection == nil || *record.At.Connection != "mock-postgres" || record.Request.UserID == nil || *record.Request.UserID != "request-actor" {
			t.Fatal("connection/request context")
		}
		for _, secret := range []string{"fixed-create-secret", "fixed-update-secret", "fixed-exec-secret"} {
			if strings.Contains(record.SQL.Raw, secret) || strings.Contains(record.SQL.Last, secret) {
				t.Fatal("stored secret")
			}
		}
	}
}
func TestPostgresTransactionErrorsAndClose(t *testing.T) {
	db, mock, store, plugin, recorder, _ := setupPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE items SET token=\$1`).WithArgs("fixed-transaction-secret").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()
	rollback := errors.New("business rollback")
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE items SET token=?", "fixed-transaction-secret").Error; err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) || plugin.Status().Captured != 1 {
		t.Fatal("executed SQL before rollback lost", err)
	}
	failure := errors.New("fixed SQL error")
	mock.ExpectExec(`UPDATE items SET token=\$1`).WithArgs("failed").WillReturnError(failure)
	if err := db.Exec("UPDATE items SET token=?", "failed").Error; !errors.Is(err, failure) {
		t.Fatal("SQL error replaced", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := db.WithContext(ctx).Exec("UPDATE items SET token=?", "canceled").Error; !errors.Is(err, context.Canceled) {
		t.Fatal("canceled SQL error replaced", err)
	}
	if len(recorder.inputs) != 1 {
		t.Fatal("failed SQL collected")
	}
	if err := plugin.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`UPDATE items SET token=\$1`).WithArgs("after close").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := db.Exec("UPDATE items SET token=?", "after close").Error; err != nil {
		t.Fatal("plugin close changed SQL", err)
	}
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 1 || len(recorder.inputs) != 1 {
		t.Fatal("closed capture changed records", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type pgValuer struct{ calls *atomic.Int64 }

func (v pgValuer) Value() (driver.Value, error) { v.calls.Add(1); return "fixed-native-value", nil }
func TestPostgresValuerNoSecondCall(t *testing.T) {
	db, mock, _, plugin, recorder, _ := setupPostgres(t)
	var calls atomic.Int64
	mock.ExpectExec(`UPDATE items SET token=\$1`).WithArgs("fixed-native-value").WillReturnResult(sqlmock.NewResult(0, 1))
	result := db.Exec("UPDATE items SET token=?", pgValuer{&calls})
	if result.Error != nil || result.RowsAffected != 1 || calls.Load() != 1 || len(recorder.inputs) != 0 || plugin.Status().Last.Kind != observer.FailureInput {
		t.Fatal("monitor re-invoked native Valuer or changed business", calls.Load(), result.Error, plugin.Status())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresFrozenRenderedInput(t *testing.T) {
	root := t.TempDir()
	os.Chmod(root, 0700)
	threshold := float64(0)
	builder, err := slowquery.New(slowquery.Config{Enabled: true, ThresholdMillis: &threshold, MaskKeys: []string{"token"}})
	if err != nil {
		t.Fatal(err)
	}
	cap := 1
	store, err := buffer.NewSlow(buffer.Config{Root: root, Scope: "frozen", DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rendered := "select 'fixed'"
	input := slowquery.Input{Raw: "select $1", RenderedSQL: &rendered, TookMillis: 1, File: "app/frozen.go", Line: 1}
	if _, ok, err := store.ReportSlowQuery(context.Background(), input, time.Now()); err != nil || !ok {
		t.Fatal(err)
	}
	snapshot := func() map[string]string {
		files := map[string]string{}
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				files[path] = string(data)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	before := snapshot()
	for _, bad := range []slowquery.Input{func() slowquery.Input { v := input; s := string([]byte{255}); v.RenderedSQL = &s; return v }(), func() slowquery.Input { v := input; v.Bindings = []any{int64(1)}; return v }()} {
		if _, ok, err := store.ReportSlowQuery(context.Background(), bad, time.Now()); ok || !errors.Is(err, slowquery.ErrInput) {
			t.Fatal("frozen invalid rendered accepted", err)
		}
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("invalid rendered input changed frozen record/overflow")
	}
}
