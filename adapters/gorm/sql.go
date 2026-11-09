package gormcapture

import (
	"gitee.com/charsen/moo-monitor-go/slowquery"
	"gorm.io/gorm"
)

// PrepareSQL is shared by the GORM callback and DB builder producers. It prepares
// only safe immutable SQL input; each producer owns its execution timing/lifecycle.
func PrepareSQL(dialect gorm.Dialector, raw string, args []any) (slowquery.Input, error) {
	input := slowquery.Input{Raw: raw}
	if dialect == nil || !supportedDialect(dialect.Name()) {
		return input, ErrUnsupported
	}
	if dialect.Name() == "sqlserver" {
		text, err := sqlserverRendered(dialect, raw, args)
		if err != nil {
			return input, err
		}
		input.RenderedSQL = &text
		return input, nil
	}
	if dialect.Name() == "mysql" {
		text, err := mysqlRendered(dialect, raw, args)
		if err != nil {
			return input, err
		}
		input.RenderedSQL = &text
		return input, nil
	}
	if dialect.Name() == "postgres" {
		text, err := postgresRendered(dialect, raw, args)
		if err != nil {
			return input, err
		}
		input.RenderedSQL = &text
		return input, nil
	}
	values, err := bindings(args)
	if err != nil {
		return input, err
	}
	input.Bindings = values
	return input, nil
}
