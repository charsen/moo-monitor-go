package dbcapture

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/goravel/framework/contracts/config"
	contractsdb "github.com/goravel/framework/contracts/database/db"
	contractsdriver "github.com/goravel/framework/contracts/database/driver"
	contractslogger "github.com/goravel/framework/contracts/database/logger"
	"github.com/goravel/framework/contracts/log"
	contractstelemetry "github.com/goravel/framework/contracts/telemetry"
	native "github.com/goravel/framework/database/db"
	nativedriver "github.com/goravel/framework/database/driver"
	nativeerrors "github.com/goravel/framework/errors"
	instrumentation "github.com/goravel/framework/telemetry/instrumentation/database"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type Factory struct {
	config   config.Config
	log      log.Log
	resolver contractstelemetry.Resolver
	owner    *Owner
	mu       sync.Mutex
	states   map[string]*connectionState
}
type connectionState struct {
	driver     contractsdriver.Driver
	gorm       *gorm.DB
	logger     contractslogger.Logger
	instrument *instrumentation.Instrument
}

// New is called by the application's lazy DB binding, not by a monitoring switch.
// Monitoring owns only its observations; the original framework owns the SQL pool.
func New(ctx context.Context, cfg config.Config, logger log.Log, resolver contractstelemetry.Resolver, connection string, monitor Config) (contractsdb.DB, *Owner, error) {
	if ctx == nil || cfg == nil || logger == nil || connection == "" {
		return nil, nil, ErrConfiguration
	}
	owner := newOwner(monitor)
	factory := &Factory{config: cfg, log: logger, resolver: resolver, owner: owner, states: map[string]*connectionState{}}
	db, err := factory.connect(ctx, connection)
	return db, owner, err
}
func (f *Factory) state(connection string) (*connectionState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if state, ok := f.states[connection]; ok {
		return state, nil
	}
	create, ok := f.config.Get("database.connections." + connection + ".via").(func() (contractsdriver.Driver, error))
	if !ok {
		return nil, nativeerrors.DatabaseConfigNotFound
	}
	driver, err := create()
	if err != nil {
		return nil, err
	}
	if driver == nil {
		return nil, nativeerrors.DatabaseConfigNotFound
	}
	logger := native.NewLogger(f.config, f.log)
	gm, instrument, err := nativedriver.BuildGorm(f.config, logger.ToGorm(), driver.Pool(), connection, f.resolver)
	if err != nil {
		return nil, err
	}
	state := &connectionState{driver: driver, gorm: gm, logger: logger, instrument: instrument}
	f.states[connection] = state
	return state, nil
}
func (f *Factory) connect(ctx context.Context, name string) (contractsdb.DB, error) {
	state, err := f.state(name)
	if err != nil {
		return nil, err
	}
	base, err := native.NewDB(ctx, f.config, state.driver, state.logger, state.gorm, state.instrument, f.resolver)
	if err != nil {
		return nil, err
	}
	if !f.owner.Status().Ready {
		return base, nil
	}
	return &observedDB{DB: base, factory: f, state: state, connection: name, ctx: ctx, connections: map[string]contractsdb.DB{}}, nil
}

type observedDB struct {
	contractsdb.DB
	factory     *Factory
	state       *connectionState
	connection  string
	ctx         context.Context
	mu          sync.Mutex
	connections map[string]contractsdb.DB
}

func (d *observedDB) supported() bool {
	return d.state.gorm != nil && d.state.gorm.Dialector != nil && (d.state.gorm.Dialector.Name() == "sqlite" || d.state.gorm.Dialector.Name() == "postgres")
}
func (d *observedDB) Table(name string) contractsdb.Query {
	if !d.supported() {
		return d.DB.Table(name)
	}
	ctx := instrumentation.ContextWithTable(d.ctx, name)
	driver := d.state.driver.Pool().Writers[0].Driver
	read, err := native.NewBuilder(d.state.gorm.Clauses(dbresolver.Read), driver)
	if err != nil {
		d.state.logger.Panicf(d.ctx, err.Error())
		return nil
	}
	write, err := native.NewBuilder(d.state.gorm.Clauses(dbresolver.Write), driver)
	if err != nil {
		d.state.logger.Panicf(d.ctx, err.Error())
		return nil
	}
	var r, w contractsdb.CommonBuilder = read, write
	if d.state.instrument != nil {
		r = instrumentation.WrapBuilder(read, d.state.instrument)
		w = instrumentation.WrapBuilder(write, d.state.instrument)
	}
	r = &observedBuilder{CommonBuilder: r, owner: d.factory.owner, dialect: d.state.gorm.Dialector, connection: d.connection}
	w = &observedBuilder{CommonBuilder: w, owner: d.factory.owner, dialect: d.state.gorm.Dialector, connection: d.connection}
	return native.NewQuery(ctx, r, w, d.state.driver.Grammar(), d.state.logger, name, nil)
}
func (d *observedDB) WithContext(ctx context.Context) contractsdb.DB {
	return &observedDB{DB: d.DB.WithContext(ctx), factory: d.factory, state: d.state, connection: d.connection, ctx: ctx, connections: map[string]contractsdb.DB{}}
}
func (d *observedDB) Connection(name string) contractsdb.DB {
	if name == "" {
		name = d.factory.config.GetString("database.default")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if db, ok := d.connections[name]; ok {
		return db
	}
	db, err := d.factory.connect(d.ctx, name)
	if err != nil {
		d.state.logger.Panicf(d.ctx, err.Error())
		return nil
	}
	d.connections[name] = db
	return db
}
func (d *observedDB) BeginTransaction() (contractsdb.Tx, error) {
	if !d.supported() {
		return d.DB.BeginTransaction()
	}
	driver := d.state.driver.Pool().Writers[0].Driver
	builder, err := native.NewTxBuilder(d.state.gorm.Clauses(dbresolver.Write), driver)
	if err != nil {
		return nil, err
	}
	observed := &observedTxBuilder{observedBuilder: &observedBuilder{CommonBuilder: builder, owner: d.factory.owner, dialect: d.state.gorm.Dialector, connection: d.connection}, native: builder}
	return native.NewTx(d.ctx, d.state.driver, d.state.logger, nil, observed, &[]native.TxLog{}, d.state.instrument), nil
}
func (d *observedDB) Transaction(callback func(contractsdb.Tx) error) (err error) {
	tx, err := d.BeginTransaction()
	if err != nil {
		return err
	}
	// This is the original public transaction convention, now using the observed Begin.
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("panic: %v", value)
			if rollback := tx.Rollback(); rollback != nil {
				err = errors.Join(err, rollback)
			}
		}
	}()
	if err = callback(tx); err != nil {
		if rollback := tx.Rollback(); rollback != nil {
			return rollback
		}
		return err
	}
	return tx.Commit()
}
func (d *observedDB) start() (time.Time, bool) {
	if !d.supported() {
		return time.Time{}, false
	}
	return d.factory.owner.start(d.ctx)
}
func (d *observedDB) finish(raw string, args []any, start time.Time, sampled bool, err error) {
	if sampled {
		d.factory.owner.complete(d.ctx, d.state.gorm.Dialector, d.connection, raw, args, start, sampled, err)
	}
}
func (d *observedDB) Delete(raw string, args ...any) (*contractsdb.Result, error) {
	start, sampled := d.start()
	result, err := d.DB.Delete(raw, args...)
	d.finish(raw, args, start, sampled, err)
	return result, err
}
func (d *observedDB) Insert(raw string, args ...any) (*contractsdb.Result, error) {
	start, sampled := d.start()
	result, err := d.DB.Insert(raw, args...)
	d.finish(raw, args, start, sampled, err)
	return result, err
}
func (d *observedDB) Update(raw string, args ...any) (*contractsdb.Result, error) {
	start, sampled := d.start()
	result, err := d.DB.Update(raw, args...)
	d.finish(raw, args, start, sampled, err)
	return result, err
}
func (d *observedDB) Select(dest any, raw string, args ...any) error {
	start, sampled := d.start()
	err := d.DB.Select(dest, raw, args...)
	d.finish(raw, args, start, sampled, err)
	return err
}
func (d *observedDB) Statement(raw string, args ...any) error {
	start, sampled := d.start()
	err := d.DB.Statement(raw, args...)
	d.finish(raw, args, start, sampled, err)
	return err
}
