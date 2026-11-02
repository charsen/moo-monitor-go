package gormcapture

import (
	"errors"
	"strings"
	"testing"

	"gitee.com/charsen/moo-go-monitor/slowquery"
	"github.com/jackc/pgx/v5"
	postgresdialect "gorm.io/driver/postgres"
)

type renderTrap struct{}

func (renderTrap) String() string { panic("must not stringify") }
func TestPostgresDisplay(t *testing.T) {
	dialect := postgresdialect.New(postgresdialect.Config{})
	vars := []any{int64(11), int64(22), int64(33), int64(44), int64(55), int64(66), int64(77), int64(88), int64(99), int64(1010)}
	rendered, err := postgresRendered(dialect, "select $1, $10, $1", vars)
	if err != nil || rendered != "select 11, 1010, 11" {
		t.Fatal("numeric order", rendered, err)
	}
	rendered, err = postgresRendered(dialect, "select '$1', $1", []any{"value"})
	if err != nil || rendered != "select ''value'', 'value'" {
		t.Fatal("mature quoted-dollar display boundary", rendered, err)
	}
	for _, mode := range []pgx.QueryExecMode{pgx.QueryExecModeCacheStatement, pgx.QueryExecModeCacheDescribe, pgx.QueryExecModeDescribeExec, pgx.QueryExecModeExec, pgx.QueryExecModeSimpleProtocol} {
		rendered, err := postgresRendered(dialect, "select $1", []any{mode, int64(42)})
		if err != nil || rendered != "select 42" {
			t.Fatal("known leading execution mode", err)
		}
	}
	for _, values := range [][]any{{pgx.QueryExecMode(99), 1}, {1, pgx.QueryExecModeSimpleProtocol}, {renderTrap{}}, {string([]byte{255})}} {
		if _, err := postgresRendered(dialect, "select $1", values); !errors.Is(err, errBinding) {
			t.Fatal("unsafe vars accepted", err)
		}
	}
	large := strings.Repeat("'", slowquery.MaxRenderedSQLBytes/4)
	if _, err := postgresRendered(dialect, "select $1,$1,$1", []any{large}); !errors.Is(err, errBinding) {
		t.Fatal("repeated escaped expansion not bounded")
	}
}
