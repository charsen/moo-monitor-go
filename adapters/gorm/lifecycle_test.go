package gormcapture_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	observer "gitee.com/charsen/moo-go-monitor/adapters/gorm"
	"gitee.com/charsen/moo-go-monitor/capture"
	"gitee.com/charsen/moo-go-monitor/slowquery"
	databaselogger "github.com/goravel/framework/contracts/database/logger"
	databasedb "github.com/goravel/framework/database/db"
	mockconfig "github.com/goravel/framework/mocks/config"
	"github.com/goravel/sqlite"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

type recorderFunc func(context.Context, slowquery.Input, time.Time) (string, bool, error)

func (f recorderFunc) ReportSlowQuery(ctx context.Context, in slowquery.Input, now time.Time) (string, bool, error) {
	return f(ctx, in, now)
}

func TestORMThresholdAndIsolation(t *testing.T) {
	filtered, q, _, _, clock := setup(t, "threshold", 200)
	p := plugin(t, filtered, "threshold", clock)
	if err := observer.Install(q, p); err != nil {
		t.Fatal(err)
	}
	if err := q.Create(&item{Name: "fast"}); err != nil {
		t.Fatal(err)
	}
	if p.Status().Filtered != 1 || p.Status().Captured != 0 {
		t.Fatal("threshold ignored", p.Status())
	}
	skipStore, skipQuery, _, _, skipClock := setup(t, "skip", 0, "skip_marker")
	skipPlugin := plugin(t, skipStore, "skip", skipClock)
	if err := observer.Install(skipQuery, skipPlugin); err != nil {
		t.Fatal(err)
	}
	if _, err := skipQuery.Exec("SELECT 'skip_marker'"); err != nil {
		t.Fatal(err)
	}
	if skipPlugin.Status().Filtered != 1 {
		t.Fatal("skip pattern ignored")
	}
	var ids []string
	for _, label := range []string{"alpha", "beta"} {
		store, q, _, _, clock := setup(t, label, 0)
		key := struct{ Label string }{label}
		p, err := observer.New(observer.Config{Connection: label, Recorder: store, Clock: clock, CaptureTimeout: time.Second, Callsite: func() (string, int) { return "request.go", 1 }, Request: func(ctx context.Context) (slowquery.Request, error) {
			id, ok := ctx.Value(key).(string)
			if !ok {
				return slowquery.Request{}, errors.New("missing explicit context")
			}
			return slowquery.Request{Method: "POST", UserID: &id}, nil
		}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { p.Close(context.Background()) })
		if err = observer.Install(q, p); err != nil {
			t.Fatal(err)
		}
		if err = q.WithContext(context.WithValue(context.Background(), key, label+"-user")).Create(&item{Name: label}); err != nil {
			t.Fatal(err)
		}
		snapshots, err := store.Snapshot(context.Background())
		if err != nil || len(snapshots) != 1 {
			t.Fatal("scope sample missing", err)
		}
		record := snapshots[0].Record
		if record.At.Connection == nil || *record.At.Connection != label || record.Request.UserID == nil || *record.Request.UserID != label+"-user" {
			t.Fatal("connection/context contaminated")
		}
		ids = append(ids, *record.Request.UserID)
	}
	if ids[0] == ids[1] {
		t.Fatal("request values shared")
	}
}

func TestORMFailureDoesNotChangeSQL(t *testing.T) {
	_, q, db, _, clock := setup(t, "failure", 0)
	var calls atomic.Int64
	recorder := recorderFunc(func(context.Context, slowquery.Input, time.Time) (string, bool, error) {
		calls.Add(1)
		return "", false, errors.New("fake-token-secret")
	})
	var seen observer.Diagnostic
	p, err := observer.New(observer.Config{Connection: "failure", Recorder: recorder, Clock: clock, OnError: func(ctx context.Context, d observer.Diagnostic) {
		seen = d
		if !capture.IsInternal(ctx) {
			t.Error("diagnostic context not marked")
		}
		// Reentrant SQL executes normally, but must not recursively capture itself.
		if err := db.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
			t.Error(err)
		}
		panic("fake-token-observer")
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(context.Background())
	if err = observer.Install(q, p); err != nil {
		t.Fatal(err)
	}
	if err = q.Create(&item{Name: "business"}); err != nil {
		t.Fatal("monitor failure changed SQL", err)
	}
	if calls.Load() != 1 || p.Status().ObserverPanics != 1 || seen.Kind != observer.FailureCapture {
		t.Fatal("diagnostic/reentry failed", p.Status())
	}
	if strings.Contains(fmt.Sprint(p.Status(), seen), "fake-token") {
		t.Fatal("diagnostic retained arbitrary cause")
	}
	var count int64
	if err := db.WithContext(capture.InternalContext(context.Background())).Raw("SELECT COUNT(*) FROM items").Row().Scan(&count); err != nil || count != 1 {
		t.Fatal("business row lost", err)
	}
}

func TestORMCloseAndConcurrentQueries(t *testing.T) {
	_, q, db, _, clock := setup(t, "concurrent", 0)
	var calls atomic.Int64
	recorder := recorderFunc(func(ctx context.Context, _ slowquery.Input, _ time.Time) (string, bool, error) {
		calls.Add(1)
		return "", true, nil
	})
	p := plugin(t, recorder, "concurrent", clock)
	if err := observer.Install(q, p); err != nil {
		t.Fatal(err)
	}
	const total = 16
	var wg sync.WaitGroup
	failures := make(chan error, total)
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			failures <- q.WithContext(context.Background()).Create(&item{Name: fmt.Sprint(i)})
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != total || p.Status().Captured != total {
		t.Fatal("concurrent events lost", p.Status())
	}
	// The public DB facade's Builder executes through sqlx, outside GORM callbacks.
	sqlxBuilder, err := databasedb.NewBuilder(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = sqlxBuilder.GetContext(context.Background(), &count, "SELECT COUNT(*) FROM items"); err != nil || count != total {
		t.Fatal("sqlx path changed", err)
	}
	if calls.Load() != total {
		t.Fatal("sqlx unexpectedly captured")
	}
}

func TestORMCloseWaitIsBounded(t *testing.T) {
	_, q, _, _, clock := setup(t, "wait", 0)
	entered, release := make(chan struct{}), make(chan struct{})
	recorder := recorderFunc(func(context.Context, slowquery.Input, time.Time) (string, bool, error) {
		close(entered)
		<-release
		return "", true, nil
	})
	p, err := observer.New(observer.Config{Connection: "wait", Recorder: recorder, Clock: clock, CaptureTimeout: 30 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err = observer.Install(q, p); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- q.Create(&item{Name: "pending"}) }()
	<-entered
	if err = p.Close(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Close did not bound wait", err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal("capture timeout changed SQL", err)
	}
	if err = p.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type emptyInstance struct{}

func (emptyInstance) Instance() *gorm.DB { return nil }
func TestORMUnsupportedAndNameCollision(t *testing.T) {
	store, q, db, _, clock := setup(t, "unsupported", 0)
	p := plugin(t, store, "unsupported", clock)
	for _, candidate := range []any{nil, struct{}{}, emptyInstance{}} {
		if err := observer.Install(candidate, p); !errors.Is(err, observer.ErrUnsupported) {
			t.Fatal("unsupported capability accepted", err)
		}
	}
	original := func(*gorm.DB) {}
	if err := db.Callback().Raw().Register("moo-monitor:slow-query:after_raw", original); err != nil {
		t.Fatal(err)
	}
	if err := observer.Install(q, p); !errors.Is(err, observer.ErrConfiguration) {
		t.Fatal("existing name accepted", err)
	}
	if db.Callback().Raw().Get("moo-monitor:slow-query:after_raw") == nil || db.Callback().Create().Get("moo-monitor:slow-query:before_create") != nil {
		t.Fatal("preflight changed callbacks")
	}
	if err := db.Exec("SELECT 1").Error; err != nil {
		t.Fatal("preflight changed business", err)
	}
}

func TestORMCaptureDeadlineAndObserverClose(t *testing.T) {
	_, q, _, _, clock := setup(t, "deadline", 0)
	observerEntered, observerRelease := make(chan struct{}), make(chan struct{})
	recorder := recorderFunc(func(ctx context.Context, _ slowquery.Input, _ time.Time) (string, bool, error) {
		<-ctx.Done()
		return "", false, ctx.Err()
	})
	p, err := observer.New(observer.Config{Connection: "deadline", Recorder: recorder, Clock: clock, CaptureTimeout: 20 * time.Millisecond, OnError: func(context.Context, observer.Diagnostic) { close(observerEntered); <-observerRelease }})
	if err != nil {
		t.Fatal(err)
	}
	if err = observer.Install(q, p); err != nil {
		t.Fatal(err)
	}
	businessCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- q.WithContext(businessCtx).Create(&item{Name: "committed"}) }()
	<-observerEntered
	if businessCtx.Err() != nil {
		t.Fatal("capture deadline canceled business context")
	}
	if err = p.Close(context.Background()); err != nil {
		t.Fatal("observer retained Close wait", err)
	}
	close(observerRelease)
	if err = <-done; err != nil {
		t.Fatal("capture deadline changed SQL result", err)
	}
	if p.Status().Failures != 1 || p.Status().Last.Kind != observer.FailureCapture {
		t.Fatal("capture timeout unobserved", p.Status())
	}
}

func TestGoravelDBFacadeIsOutsideGORM(t *testing.T) {
	store, q, db, _, clock := setup(t, "db-facade", 0)
	p := plugin(t, store, "db-facade", clock)
	if err := observer.Install(q, p); err != nil {
		t.Fatal(err)
	}
	if err := q.Create(&item{Name: "ORM"}); err != nil {
		t.Fatal(err)
	}
	before := p.Status().Captured
	cfg := mockconfig.NewConfig(t)
	cfg.On("Get", mock.Anything).Return(nil)
	cfg.On("GetString", mock.Anything).Return("")
	cfg.On("GetBool", mock.Anything).Return(false)
	driver := sqlite.NewSqlite(cfg, nil, "fixture")
	nativeLogger := (&databasedb.Logger{}).Level(databaselogger.Silent)
	facade, err := databasedb.NewDB(context.Background(), cfg, driver, nativeLogger, db, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	count, err := facade.Table("items").Count()
	if err != nil || count != 1 {
		t.Fatal("public DB facade query changed", err)
	}
	if p.Status().Captured != before {
		t.Fatal("sqlx DB facade unexpectedly used GORM plugin")
	}
}
