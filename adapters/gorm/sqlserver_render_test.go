package gormcapture

import (
	"errors"
	"strings"
	"testing"

	"gitee.com/charsen/moo-go-monitor/slowquery"
	sqlserverdialect "gorm.io/driver/sqlserver"
)

func TestSQLServerDisplay(t *testing.T) {
	dialect := sqlserverdialect.New(sqlserverdialect.Config{})
	args := []any{true, int64(22), int64(33), int64(44), int64(55), int64(66), int64(77), int64(88), int64(99), int64(1010)}
	text, err := sqlserverRendered(dialect, "select @p1, @p10, @p1", args)
	if err != nil || text != "select 1, 1010, 1" || args[0] != true {
		t.Fatal("numbered display or caller mutation", text, err, args[0])
	}
	text, err = sqlserverRendered(dialect, "select '@p1', @p1", []any{"value"})
	if err != nil || text != "select ''value'', 'value'" {
		t.Fatal("mature quoted-placeholder boundary", text, err)
	}
	for _, args := range [][]any{{renderTrap{}}, {string([]byte{255})}} {
		if _, err = sqlserverRendered(dialect, "select @p1", args); !errors.Is(err, errBinding) {
			t.Fatal("unsafe vars accepted", err)
		}
	}
	large := strings.Repeat("'", slowquery.MaxRenderedSQLBytes/4)
	if _, err = sqlserverRendered(dialect, "select @p1,@p1,@p1", []any{large}); !errors.Is(err, errBinding) {
		t.Fatal("repeated escaped expansion unbounded", err)
	}
}
