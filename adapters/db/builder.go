package dbcapture

import (
	"context"
	"database/sql"

	contractsdb "github.com/goravel/framework/contracts/database/db"
	"github.com/jmoiron/sqlx"
	"gorm.io/gorm"
)

type observedBuilder struct {
	contractsdb.CommonBuilder
	owner      *Owner
	dialect    gorm.Dialector
	connection string
}

func (b *observedBuilder) ExecContext(ctx context.Context, raw string, args ...any) (sql.Result, error) {
	start, sampled := b.owner.start(ctx)
	result, err := b.CommonBuilder.ExecContext(ctx, raw, args...)
	b.owner.complete(ctx, b.dialect, b.connection, raw, args, start, sampled, err)
	return result, err
}
func (b *observedBuilder) GetContext(ctx context.Context, dest any, raw string, args ...any) error {
	start, sampled := b.owner.start(ctx)
	err := b.CommonBuilder.GetContext(ctx, dest, raw, args...)
	b.owner.complete(ctx, b.dialect, b.connection, raw, args, start, sampled, err)
	return err
}
func (b *observedBuilder) SelectContext(ctx context.Context, dest any, raw string, args ...any) error {
	start, sampled := b.owner.start(ctx)
	err := b.CommonBuilder.SelectContext(ctx, dest, raw, args...)
	b.owner.complete(ctx, b.dialect, b.connection, raw, args, start, sampled, err)
	return err
}
func (b *observedBuilder) QueryxContext(ctx context.Context, raw string, args ...any) (*sqlx.Rows, error) {
	start, sampled := b.owner.start(ctx)
	rows, err := b.CommonBuilder.QueryxContext(ctx, raw, args...)
	b.owner.complete(ctx, b.dialect, b.connection, raw, args, start, sampled, err)
	return rows, err
}

// Explain is inherited unchanged; it is never the source of the captured raw identity.

type observedTxBuilder struct {
	*observedBuilder
	native contractsdb.TxBuilder
}

func (b *observedTxBuilder) Commit() error   { return b.native.Commit() }
func (b *observedTxBuilder) Rollback() error { return b.native.Rollback() }
