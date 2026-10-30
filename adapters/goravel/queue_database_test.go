package goravel_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	adapter "gitee.com/charsen/moo-go-monitor/adapters/goravel"
	"github.com/goravel/framework/contracts/database/driver"
	contractslog "github.com/goravel/framework/contracts/log"
	contractsqueue "github.com/goravel/framework/contracts/queue"
	frameworkdb "github.com/goravel/framework/database/db"
	databasedriver "github.com/goravel/framework/database/driver"
	"github.com/goravel/framework/foundation"
	frameworkjson "github.com/goravel/framework/foundation/json"
	mockcache "github.com/goravel/framework/mocks/cache"
	mockconfig "github.com/goravel/framework/mocks/config"
	mockqueue "github.com/goravel/framework/mocks/queue"
	frameworkqueue "github.com/goravel/framework/queue"
	"github.com/goravel/sqlite"
	"github.com/stretchr/testify/mock"
)

type quietQueueLog struct{ contractslog.Log }

func (l *quietQueueLog) WithContext(context.Context) contractslog.Log { return l }
func (*quietQueueLog) Error(...any)                                   {}
func (*quietQueueLog) Errorf(string, ...any)                          {}
func (*quietQueueLog) Warningf(string, ...any)                        {}
func (*quietQueueLog) Infof(string, ...any)                           {}
func TestQueueMatureDatabaseDriver(t *testing.T) {
	collector, store := setup(t)
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(root, "queue.sqlite")
	cfg := mockconfig.NewConfig(t)
	cfg.On("Get", "database.connections.fixture14.via").Return(func() (driver.Driver, error) { return sqlite.NewSqlite(cfg, &quietQueueLog{}, "fixture14"), nil })
	cfg.On("Get", mock.Anything).Return(nil).Maybe()
	cfg.On("GetString", "database.connections.fixture14.database").Return(databasePath)
	cfg.On("GetString", mock.Anything).Return("").Maybe()
	cfg.On("GetBool", mock.Anything).Return(false).Maybe()
	cfg.On("GetInt", mock.Anything, mock.Anything).Return(200).Maybe()
	cfg.On("GetDuration", mock.Anything, mock.Anything).Return(time.Duration(3600)).Maybe()
	// Only the actual driver provider's Register is needed for its JSON grammar.
	(&sqlite.ServiceProvider{}).Register(foundation.NewApplication())
	nativeLog := &quietQueueLog{}
	nativeDriver := sqlite.NewSqlite(cfg, nativeLog, "fixture14")
	nativeLogger := frameworkdb.NewLogger(cfg, nativeLog)
	gormDB, instrument, err := databasedriver.BuildGorm(cfg, nativeLogger.ToGorm(), nativeDriver.Pool(), "fixture14", nil)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := frameworkdb.NewDB(context.Background(), cfg, nativeDriver, nativeLogger, gormDB, instrument, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{
		"CREATE TABLE jobs(id INTEGER PRIMARY KEY AUTOINCREMENT,queue TEXT,payload TEXT,attempts INTEGER DEFAULT 0,reserved_at TIMESTAMP NULL,available_at TIMESTAMP NULL,created_at TIMESTAMP NULL)",
		"CREATE TABLE failed_jobs(id INTEGER PRIMARY KEY AUTOINCREMENT,uuid TEXT,connection TEXT,queue TEXT,payload TEXT,exception TEXT,failed_at TIMESTAMP NULL)",
	} {
		if _, err = sqlDB.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	jobs := frameworkqueue.NewJobStorer()
	var calls atomic.Int64
	job := &queueJob{name: "database-final", handle: func(...any) error { calls.Add(1); return errors.New("database final fixture") }}
	jobs.Register([]contractsqueue.Job{job})
	config := mockqueue.NewConfig(t)
	config.On("Driver", "fixture").Return(contractsqueue.DriverDatabase).Once()
	config.On("GetString", "queue.connections.fixture.connection").Return("fixture14")
	config.On("GetString", "queue.connections.fixture.table", "jobs").Return("jobs")
	config.On("GetInt", "queue.connections.fixture.retry_after", 60).Return(60)
	config.On("Debug").Return(false)
	config.On("FailedDatabase").Return("fixture14")
	config.On("FailedTable").Return("failed_jobs")
	cache := mockcache.NewCache(t)
	lock := mockcache.NewLock(t)
	cache.On("Lock", mock.Anything, time.Minute).Return(lock)
	lock.On("Block", time.Minute).Return(true)
	lock.On("Release").Return(true)
	json := frameworkjson.New()
	// Seed via the mature Driver itself, then let the owner construct its one backend.
	backend, err := frameworkqueue.NewDatabase(config, cache, db, jobs, json, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err = backend.Push(contractsqueue.Task{UUID: "database-task", ChainJob: contractsqueue.ChainJob{Job: job}}, "default"); err != nil {
		t.Fatal(err)
	}
	worker, err := adapter.NewQueueWorker(adapter.WorkerDependencies{Config: config, Cache: cache, DB: db, Jobs: jobs, JSON: json, Log: nativeLog}, adapter.WorkerConfig{Connection: "fixture", Queue: "default", Concurrent: 1, Tries: 2, Collector: collector})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- worker.Run() }()
	until := time.Now().Add(5 * time.Second)
	for calls.Load() < 2 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 2 {
		t.Fatal("mature backend did not execute attempts")
	}
	if err = worker.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	var queued, failed int
	if err = sqlDB.QueryRow("SELECT COUNT(*) FROM jobs").Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if err = sqlDB.QueryRow("SELECT COUNT(*) FROM failed_jobs").Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if queued != 0 || failed != 1 || totalRuntime(t, store) != 1 {
		t.Fatal("native deletion/failed persistence or monitor terminal count changed", queued, failed)
	}
}
