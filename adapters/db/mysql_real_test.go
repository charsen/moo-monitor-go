//go:build integration

package dbcapture

import (
	"context"
	"os"
	"testing"
	"time"

	"gitee.com/charsen/moo-go-monitor/buffer"
	"gitee.com/charsen/moo-go-monitor/slowquery"
	frameworkconfig "github.com/goravel/framework/config"
	contractsdriver "github.com/goravel/framework/contracts/database/driver"
	nativedriver "github.com/goravel/framework/database/driver"
	"github.com/goravel/framework/foundation"
	"github.com/goravel/mysql"
)

func TestMySQLRealDBFacadeObservation(t *testing.T) {
	(&mysql.ServiceProvider{}).Register(foundation.NewApplication())
	root := t.TempDir()
	os.Chmod(root, 0700)
	empty := root + "/empty.env"
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := frameworkconfig.NewApplication(empty)
	cfg.Add("app", map[string]any{"debug": true})
	sink := &memoryLog{}
	var driver contractsdriver.Driver
	cfg.Add("database", map[string]any{"default": "mysql-real", "slow_threshold": 0, "connections": map[string]any{"mysql-real": map[string]any{
		"database": "moo_monitor_integration", "host": "127.0.0.1", "port": 3307, "username": "root", "password": "", "charset": "utf8mb4",
		"via": func() (contractsdriver.Driver, error) {
			driver = mysql.NewMysql(cfg, sink, nil, "mysql-real")
			return driver, nil
		},
	}}})
	defer nativedriver.CloseConnections()

	zero := float64(0)
	builder, err := slowquery.New(slowquery.Config{Enabled: true, ThresholdMillis: &zero, MaskKeys: []string{"token"}})
	if err != nil {
		t.Fatal(err)
	}
	cap := 0
	store, err := buffer.NewSlow(buffer.Config{Root: root, Scope: "mysql-real-facade", DailyCap: &cap}, builder)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, owner, err := New(context.Background(), cfg, sink, nil, "mysql-real", Config{Enabled: true, Recorder: store, Clock: time.Now, Callsite: func() (string, int) { return "app/mysql_real_test.go", 1 }})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	if err = db.Statement("CREATE TABLE IF NOT EXISTS monitor_facade_items (id BIGINT PRIMARY KEY, token VARCHAR(64))"); err != nil {
		t.Fatal(err)
	}
	defer db.Statement("DROP TABLE IF EXISTS monitor_facade_items")
	if _, err = db.Table("monitor_facade_items").Insert(map[string]any{"id": int64(1), "token": "real-facade-secret"}); err != nil {
		t.Fatal(err)
	}
	var values []struct{ ID int64 }
	if err = db.Select(&values, "SELECT id FROM monitor_facade_items"); err != nil || len(values) != 1 {
		t.Fatal("real facade select", err, len(values))
	}
	rows, err := store.ReadOnlySnapshot(context.Background())
	if err != nil || len(rows) == 0 {
		t.Fatal("real MySQL DB facade slow query was not captured", err, len(rows))
	}
	var original bool
	for _, row := range rows {
		if row.Record.SQL.Raw != "" && row.Record.At.Connection != nil && *row.Record.At.Connection == "mysql-real" {
			original = true
		}
	}
	if !original {
		t.Fatal("missing original SQL/connection")
	}
}
