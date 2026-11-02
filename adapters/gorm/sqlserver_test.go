package gormcapture_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"os"
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
	sqlserverdialect "gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type mssqlRequestKey struct{}

type mssqlItem struct {
	ID    int64
	Token string
}

func (mssqlItem) TableName() string { return "items" }

type mssqlTrace struct{ calls atomic.Int64 }

func (p *mssqlTrace) LogMode(gormlogger.LogLevel) gormlogger.Interface { return p }
func (*mssqlTrace) Info(context.Context, string, ...any)               {}
func (*mssqlTrace) Warn(context.Context, string, ...any)               {}
func (*mssqlTrace) Error(context.Context, string, ...any)              {}

// Count native delivery without choosing to format values in the application's logger.
func (p *mssqlTrace) Trace(context.Context, time.Time, func() (string, int64), error) { p.calls.Add(1) }

type mssqlRecorder struct {
	store  *buffer.SlowBuffer
	mu     sync.Mutex
	inputs []slowquery.Input
}

func (p *mssqlRecorder) ReportSlowQuery(ctx context.Context, input slowquery.Input, now time.Time) (string, bool, error) {
	p.mu.Lock()
	p.inputs = append(p.inputs, input)
	p.mu.Unlock()
	return p.store.ReportSlowQuery(ctx, input, now)
}
func setupSQLServer(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *buffer.SlowBuffer, *observer.Plugin, *mssqlRecorder, *mssqlTrace) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	native := &mssqlTrace{}
	db, err := gorm.Open(sqlserverdialect.New(sqlserverdialect.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true, DisableAutomaticPing: true, Logger: native})
	if err != nil {
		t.Fatal(err)
	}
	db = db.WithContext(context.WithValue(context.Background(), mssqlRequestKey{}, "request-actor"))
	root := t.TempDir()
	os.Chmod(root, 0700)
	threshold := float64(0)
	builder, err := slowquery.New(slowquery.Config{Enabled: true, ThresholdMillis: &threshold, MaskKeys: []string{"token"}, Env: "testing", Project: "mssql-mock"})
	if err != nil {
		t.Fatal(err)
	}
	cap := 0
	store, err := buffer.NewSlow(buffer.Config{Root: root, Scope: "mssql", DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	recorder := &mssqlRecorder{store: store}
	var ticks atomic.Int64
	plugin, err := observer.New(observer.Config{Connection: "mock-sqlserver", Recorder: recorder, Clock: func() time.Time {
		return time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC).Add(time.Duration(ticks.Add(1)) * time.Millisecond)
	}, Callsite: func() (string, int) { return "app/mock-mssql.go", 31 }, Request: func(ctx context.Context) (slowquery.Request, error) {
		id, ok := ctx.Value(mssqlRequestKey{}).(string)
		if !ok {
			return slowquery.Request{Method: "CLI"}, nil
		}
		return slowquery.Request{Method: "POST", UserID: &id}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { plugin.Close(context.Background()) })
	query := frameworkgorm.NewQuery(context.Background(), nil, database.Config{Connection: "mock-sqlserver", Driver: "sqlserver"}, db, nil, nil, nil, nil, nil)
	if err = observer.Install(query, plugin); err != nil {
		t.Fatal(err)
	}
	return db, mock, store, plugin, recorder, native
}
func TestSQLServerCallbacks(t *testing.T) {
	db, mock, store, plugin, recorder, native := setupSQLServer(t)
	mock.ExpectQuery(`INSERT INTO "items"`).WithArgs("fixed-create-secret").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(17))
	item := mssqlItem{Token: "fixed-create-secret"}
	if err := db.Create(&item).Error; err != nil || item.ID != 17 {
		t.Fatal("create callback", err)
	}
	mock.ExpectQuery(`SELECT \* FROM "items" WHERE id = @p1`).WithArgs(int64(17)).WillReturnRows(sqlmock.NewRows([]string{"id", "token"}).AddRow(17, "fixed-create-secret"))
	var rows []mssqlItem
	if err := db.WithContext(context.WithValue(context.Background(), mssqlRequestKey{}, "request-actor")).Where("id = ?", int64(17)).Find(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatal("query callback", err)
	}
	mock.ExpectExec(`UPDATE "items" SET "token"=@p1 WHERE id = @p2`).WithArgs("fixed-update-secret", int64(17)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := db.Model(&mssqlItem{}).Where("id = ?", int64(17)).Update("token", "fixed-update-secret").Error; err != nil {
		t.Fatal("update callback", err)
	}
	mock.ExpectExec(`DELETE FROM "items" WHERE id = @p1`).WithArgs(int64(17)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := db.Where("id = ?", int64(17)).Delete(&mssqlItem{}).Error; err != nil {
		t.Fatal("delete callback", err)
	}
	mock.ExpectQuery(`SELECT @p1 AS n`).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(42))
	var number int64
	if err := db.Raw("SELECT ? AS n", int64(42)).Row().Scan(&number); err != nil || number != 42 {
		t.Fatal("row callback", err)
	}
	mock.ExpectExec(`UPDATE items SET token=@p1 WHERE id=@p2`).WithArgs("fixed-exec-secret", int64(17)).WillReturnResult(sqlmock.NewResult(0, 1))
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
		if input.RenderedSQL == nil || len(input.Bindings) != 0 || !strings.Contains(input.Raw, "@p") {
			t.Fatal("raw/display boundary")
		}
	}
	for _, snapshot := range snapshots {
		record := snapshot.Record
		if record.At.Connection == nil || *record.At.Connection != "mock-sqlserver" || record.Request.UserID == nil || *record.Request.UserID != "request-actor" {
			t.Fatal("connection/request context")
		}
		for _, secret := range []string{"fixed-create-secret", "fixed-update-secret", "fixed-exec-secret"} {
			if strings.Contains(record.SQL.Raw, secret) || strings.Contains(record.SQL.Last, secret) {
				t.Fatal("stored secret")
			}
		}
	}
}
func TestSQLServerTransactionErrorsAndClose(t *testing.T) {
	db, mock, store, plugin, recorder, _ := setupSQLServer(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE items SET token=@p1`).WithArgs("fixed-transaction-secret").WillReturnResult(sqlmock.NewResult(0, 1))
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
	mock.ExpectExec(`UPDATE items SET token=@p1`).WithArgs("failed").WillReturnError(failure)
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
	mock.ExpectExec(`UPDATE items SET token=@p1`).WithArgs("after close").WillReturnResult(sqlmock.NewResult(0, 1))
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

type mssqlValuer struct{ calls *atomic.Int64 }

func (v mssqlValuer) Value() (driver.Value, error) { v.calls.Add(1); return "fixed-native-value", nil }
func TestSQLServerValuerNoSecondCall(t *testing.T) {
	db, mock, _, plugin, recorder, _ := setupSQLServer(t)
	var calls atomic.Int64
	mock.ExpectExec(`UPDATE items SET token=@p1`).WithArgs("fixed-native-value").WillReturnResult(sqlmock.NewResult(0, 1))
	result := db.Exec("UPDATE items SET token=?", mssqlValuer{&calls})
	if result.Error != nil || result.RowsAffected != 1 || calls.Load() != 1 || len(recorder.inputs) != 0 || plugin.Status().Last.Kind != observer.FailureInput {
		t.Fatal("monitor re-invoked native Valuer or changed business", calls.Load(), result.Error, plugin.Status())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
