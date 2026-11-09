package dbcapture

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/slowquery"
	"github.com/goravel/framework/contracts/database"
	"github.com/goravel/framework/contracts/database/driver"
	"github.com/goravel/framework/contracts/testing/docker"
	frameworkdb "github.com/goravel/framework/database/db"
	"github.com/goravel/framework/foundation"
	frameworkloggermocks "github.com/goravel/framework/mocks/database/logger"
	"github.com/goravel/sqlite"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	(&sqlite.ServiceProvider{}).Register(foundation.NewApplication())
	os.Exit(m.Run())
}

type fixtureDriver struct{ grammar driver.Grammar }

func (f fixtureDriver) Pool() database.Pool {
	return database.Pool{Writers: []database.Config{{Driver: "sqlite"}}}
}
func (f fixtureDriver) Grammar() driver.Grammar                { return f.grammar }
func (f fixtureDriver) Processor() driver.Processor            { return nil }
func (f fixtureDriver) Docker() (docker.DatabaseDriver, error) { return nil, nil }
func TestOriginalQueryAndTxObserved(t *testing.T) {
	root := t.TempDir()
	os.Chmod(root, 0700)
	gm, err := gorm.Open(sqlite.Open(root+"/fixture.sqlite"), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gm.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err = gm.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, token TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	threshold := float64(0)
	builder, err := slowquery.New(slowquery.Config{Enabled: true, ThresholdMillis: &threshold, MaskKeys: []string{"token"}})
	if err != nil {
		t.Fatal(err)
	}
	cap := 0
	store, err := buffer.NewSlow(buffer.Config{Root: root, Scope: "db-query", DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var ticks atomic.Int64
	owner := newOwner(Config{Enabled: true, Recorder: store, Clock: func() time.Time {
		return time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC).Add(time.Duration(ticks.Add(1)) * time.Millisecond)
	}, Callsite: func() (string, int) { return "app/db-fixture.go", 1 }})
	defer owner.Close(context.Background())
	native, err := frameworkdb.NewBuilder(gm, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	observed := &observedBuilder{CommonBuilder: native, owner: owner, dialect: gm.Dialector, connection: "fixture"}
	grammar := sqlite.NewGrammar(nil, "")
	logger := frameworkloggermocks.NewLogger(t)
	logger.On("Trace", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	query := frameworkdb.NewQuery(context.Background(), observed, observed, grammar, logger, "items", nil)
	if _, err = query.Insert(map[string]any{"id": int64(1), "token": "fixed-insert-secret"}); err != nil {
		t.Fatal(err)
	}
	var values []struct {
		ID    int64
		Token string
	}
	if err = query.Where("id", 1).Get(&values); err != nil || len(values) != 1 {
		t.Fatal("native query chain", err)
	}
	txBuilder, err := frameworkdb.NewTxBuilder(gm, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &observedTxBuilder{observedBuilder: &observedBuilder{CommonBuilder: txBuilder, owner: owner, dialect: gm.Dialector, connection: "fixture"}, native: txBuilder}
	tx := frameworkdb.NewTx(context.Background(), fixtureDriver{grammar}, logger, nil, wrapped, &[]frameworkdb.TxLog{}, nil)
	if _, err = tx.Table("items").Where("id", 1).Update("token", "fixed-transaction-secret"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if owner.Status().Captured != 3 {
		t.Fatal("query and tx count", owner.Status())
	}
	if err = gm.Raw("SELECT token FROM items WHERE id=1").Scan(&values[0].Token).Error; err != nil || values[0].Token != "fixed-insert-secret" {
		t.Fatal("rollback replaced", err)
	}
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 3 {
		t.Fatal(err, len(snapshots))
	}
	for _, snapshot := range snapshots {
		if strings.Contains(snapshot.Record.SQL.Last, "secret") {
			t.Fatal("unmasked SQL")
		}
	}
}
