package gormcapture

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gitee.com/charsen/moo-monitor-go/slowquery"
	"github.com/goravel/framework/support/carbon"
	mysqldialect "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestMySQLDisplay(t *testing.T) {
	dialect := mysqldialect.New(mysqldialect.Config{})
	when := time.Date(2026, 11, 2, 3, 4, 5, 0, time.UTC)
	date := carbon.DateTime{Carbon: carbon.NewDateTime(carbon.FromStdTime(when)).Carbon}
	input, err := PrepareSQL(dialect, "select ?, ?, ?, ?, ?", []any{"a'b", true, []byte("utf8"), date, gorm.DeletedAt{}})
	if err != nil || input.Raw != "select ?, ?, ?, ?, ?" || input.RenderedSQL == nil || len(input.Bindings) != 0 {
		t.Fatal(input, err)
	}
	if !strings.Contains(*input.RenderedSQL, "a''b") || !strings.Contains(*input.RenderedSQL, "2026-11-02 03:04:05") || !strings.Contains(*input.RenderedSQL, "NULL") {
		t.Fatal(*input.RenderedSQL)
	}
	for _, values := range [][]any{{renderTrap{}}, {string([]byte{255})}, {strings.Repeat("'", slowquery.MaxRenderedSQLBytes/2)}} {
		if _, err := PrepareSQL(dialect, "select ?", values); !errors.Is(err, errBinding) {
			t.Fatal("unsafe display accepted", err)
		}
	}
}
