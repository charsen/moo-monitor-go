//go:build integration

package gormcapture_test

import (
	"context"
	"os"
	"testing"
	"time"

	observer "gitee.com/charsen/moo-monitor-go/adapters/gorm"
	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/slowquery"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type realGORMInstance struct{ db *gorm.DB }

func (i realGORMInstance) Instance() *gorm.DB { return i.db }

// TestMySQLRealSlowQuery proves the GORM entry against an actual MySQL
// instance. It is opt-in so the default suite stays hermetic:
//
//	MOO_MYSQL_DSN='root@tcp(127.0.0.1:3307)/moo_monitor_test?parseTime=true' \
//	  go test -tags=integration ./adapters/gorm -run TestMySQLRealSlowQuery
func TestMySQLRealSlowQuery(t *testing.T) {
	dsn := os.Getenv("MOO_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MOO_MYSQL_DSN is not set")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("CREATE TABLE IF NOT EXISTS monitor_gorm_items (id BIGINT PRIMARY KEY, token VARCHAR(64))").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DROP TABLE IF EXISTS monitor_gorm_items").Error })
	if err = db.Exec("INSERT INTO monitor_gorm_items (id, token) VALUES (1, 'real-secret')").Error; err != nil {
		t.Fatal(err)
	}

	zero := float64(0)
	builder, err := slowquery.New(slowquery.Config{Enabled: true, ThresholdMillis: &zero, MaskKeys: []string{"token"}, Env: "integration", Project: "mysql-real"})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	os.Chmod(root, 0700)
	cap := 0
	store, err := buffer.NewSlow(buffer.Config{Root: root, Scope: "mysql-real", DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	plugin, err := observer.New(observer.Config{Connection: "mysql-real", Recorder: store, CaptureTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err = observer.Install(realGORMInstance{db: db}, plugin); err != nil {
		t.Fatal(err)
	}
	defer plugin.Close(context.Background())

	if err = db.WithContext(context.Background()).Exec("SELECT COUNT(*) FROM monitor_gorm_items").Error; err != nil {
		t.Fatal(err)
	}
	rows, err := store.ReadOnlySnapshot(context.Background())
	if err != nil || len(rows) == 0 {
		t.Fatal("real MySQL slow query was not captured", err, len(rows))
	}
	if rows[0].Record.SQL.Raw == "" || rows[0].Record.SQL.Last == "" {
		t.Fatal("captured SQL is empty")
	}
}
