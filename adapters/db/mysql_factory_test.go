package dbcapture

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/slowquery"
	"github.com/DATA-DOG/go-sqlmock"
	frameworkconfig "github.com/goravel/framework/config"
	"github.com/goravel/framework/contracts/database"
	contractsdb "github.com/goravel/framework/contracts/database/db"
	contractsdriver "github.com/goravel/framework/contracts/database/driver"
	native "github.com/goravel/framework/database/db"
	nativedriver "github.com/goravel/framework/database/driver"
	"github.com/goravel/framework/foundation"
	"github.com/goravel/mysql"
	mysqldialect "gorm.io/driver/mysql"
)

type mysqlPoolDriver struct {
	fixtureDriver
	pool database.Pool
}

func (d mysqlPoolDriver) Pool() database.Pool { return d.pool }

func TestMySQLFactoryNativePoolAndMethods(t *testing.T) {
	(&mysql.ServiceProvider{}).Register(foundation.NewApplication())
	root := t.TempDir()
	os.Chmod(root, 0700)
	empty := filepath.Join(root, "empty.env")
	os.WriteFile(empty, nil, 0600)
	cfg := frameworkconfig.NewApplication(empty)
	cfg.Add("app", map[string]any{"debug": true})
	sink := &memoryLog{}
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	var constructions atomic.Int64
	var driver mysqlPoolDriver
	cfg.Add("database", map[string]any{"default": "mysql40", "slow_threshold": 200, "connections": map[string]any{"mysql40": map[string]any{"database": "fixture", "host": "invalid.invalid", "port": 1, "via": func() (contractsdriver.Driver, error) { constructions.Add(1); return driver, nil }}}})
	// Real official configuration/Pool; replace only the transport dialector before opening.
	// Grammar's production version probe would dial a service, so use the public fixed-version grammar.
	poolConfig := mysql.NewMysql(cfg, sink, nil, "mysql40").Pool()
	poolConfig.Writers[0].Dialector = mysqldialect.New(mysqldialect.Config{Conn: sqlDB, SkipInitializeWithVersion: true})
	driver = mysqlPoolDriver{fixtureDriver: fixtureDriver{grammar: mysql.NewGrammar("fixture", "", "8.0.36", mysql.Name)}, pool: poolConfig}
	defer nativedriver.CloseConnections()
	// Open through Goravel's native path first. Factory must borrow that exact pool.
	gm, instrument, err := nativedriver.BuildGorm(cfg, native.NewLogger(cfg, sink).ToGorm(), driver.Pool(), "mysql40", nil)
	if err != nil {
		t.Fatal(err)
	}
	threshold := float64(0)
	builder, _ := slowquery.New(slowquery.Config{Enabled: true, ThresholdMillis: &threshold, MaskKeys: []string{"token"}})
	cap := 0
	store, err := buffer.NewSlow(buffer.Config{Root: root, Scope: "mysql40", DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var tick atomic.Int64
	db, owner, err := New(context.WithValue(context.Background(), actorKey{}, "first"), cfg, sink, nil, "mysql40", Config{Enabled: true, Recorder: store, Clock: func() time.Time {
		return time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC).Add(time.Duration(tick.Add(1)) * time.Millisecond)
	}, Callsite: func() (string, int) { return "app/mysql40.go", 40 }, Request: func(ctx context.Context) (slowquery.Request, error) {
		id, _ := ctx.Value(actorKey{}).(string)
		return slowquery.Request{Method: "CLI", UserID: &id}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	observed := db.(*observedDB)
	pool, err := observed.state.gorm.DB()
	if err != nil || pool != sqlDB || observed.state.gorm != gm || observed.state.instrument != instrument || constructions.Load() != 1 {
		t.Fatal("native pool replaced", err)
	}
	before := sink.traces.Load()
	mock.ExpectExec("INSERT INTO items").WithArgs(int64(1), "fixed-insert-secret").WillReturnResult(sqlmock.NewResult(1, 1))
	if _, err = db.Table("items").Insert(map[string]any{"id": int64(1), "token": "fixed-insert-secret"}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`INSERT INTO items`).WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(2, 1))
	if _, err = db.Insert("INSERT INTO items(id) VALUES (?)", int64(2)); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`UPDATE items`).WithArgs("fixed-update-secret", int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err = db.Update("UPDATE items SET token=? WHERE id=?", "fixed-update-secret", int64(1)); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`DELETE FROM items`).WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	if _, err = db.Delete("DELETE FROM items WHERE id=?", int64(2)); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT id FROM items`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	var rows []struct{ ID int64 }
	if err = db.Select(&rows, "SELECT id FROM items WHERE id=?", int64(1)); err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	mock.ExpectExec(`UPDATE items`).WithArgs("fixed-statement-secret").WillReturnResult(sqlmock.NewResult(0, 1))
	if err = db.Statement("UPDATE items SET token=?", "fixed-statement-secret"); err != nil {
		t.Fatal(err)
	}
	if owner.Status().Captured != 6 || sink.traces.Load() != before+6 {
		t.Fatal("native trace/capture count", owner.Status(), sink.traces.Load()-before)
	}
	current := db.WithContext(context.WithValue(context.Background(), actorKey{}, "second")).Connection("mysql40")
	if current.Connection("mysql40") != current.Connection("mysql40") {
		t.Fatal("connection cache")
	}
	if constructions.Load() != 1 {
		t.Fatal("second pool")
	}
	mock.ExpectBegin()
	tx, err := current.BeginTransaction()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("UPDATE items SET token = \\? WHERE id = \\?").WithArgs("fixed-tx-secret", 1).WillReturnResult(sqlmock.NewResult(0, 1))
	before = sink.traces.Load()
	if _, err = tx.Table("items").Where("id", 1).Update("token", "fixed-tx-secret"); err != nil {
		t.Fatal(err)
	}
	if sink.traces.Load() != before {
		t.Fatal("transaction trace delivered early")
	}
	mock.ExpectCommit()
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if sink.traces.Load() != before+1 {
		t.Fatal("commit trace count")
	}
	rollback := errors.New("business rollback")
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE items`).WithArgs("fixed-rollback-secret").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()
	if err = current.Transaction(func(tx contractsdb.Tx) error {
		if e := tx.Statement("UPDATE items SET token=?", "fixed-rollback-secret"); e != nil {
			return e
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	failure := errors.New("native SQL failure")
	mock.ExpectExec(`UPDATE items`).WillReturnError(failure)
	if err = current.Statement("UPDATE items SET token=?", "failure"); !errors.Is(err, failure) {
		t.Fatal("native error replaced", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err = current.WithContext(canceled).Statement("UPDATE items SET token=?", "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatal("context lost", err)
	}
	if owner.Status().Captured != 8 {
		t.Fatal("failed SQL counted", owner.Status())
	}
	snapshots, err := store.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seenSecond := false
	for _, s := range snapshots {
		if s.Record.Request.UserID != nil && *s.Record.Request.UserID == "second" {
			seenSecond = true
		}
		if strings.Contains(s.Record.SQL.Last, "secret") {
			t.Fatal("stored secret")
		}
	}
	if !seenSecond {
		t.Fatal("WithContext identity lost")
	}
	if err = owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`UPDATE items`).WithArgs("after close").WillReturnResult(sqlmock.NewResult(0, 1))
	if err = current.Statement("UPDATE items SET token=?", "after close"); err != nil || owner.Status().Captured != 8 {
		t.Fatal("close changed business", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
