package gormcapture

import (
	"unicode/utf8"

	"gitee.com/charsen/moo-monitor-go/slowquery"
	"gorm.io/gorm"
)

// mysqlRendered uses the locked dialect only after copying supported values.
// The result is bounded display text; Raw still owns the record identity.
func mysqlRendered(dialect gorm.Dialector, raw string, args []any) (string, error) {
	limit := slowquery.MaxRenderedSQLBytes
	if len(raw) > limit || !utf8.ValidString(raw) || len(args) > limit/16 {
		return "", errBinding
	}
	values, err := bindings(args)
	if err != nil {
		return "", err
	}
	remaining := limit - len(raw)
	// Explain consumes each value at most once for MySQL's sequential placeholders.
	// Count every supplied value conservatively, including unused values.
	for _, value := range values {
		cost := 400
		switch v := value.(type) {
		case string:
			if len(v) > (limit-2)/2 {
				return "", errBinding
			}
			cost = 2*len(v) + 2
		case nil:
			cost = 4
		case bool:
			cost = 5
		case int, int64:
			cost = 24
		}
		if cost > remaining {
			return "", errBinding
		}
		remaining -= cost
	}
	rendered := dialect.Explain(raw, values...)
	if rendered == "" || len(rendered) > limit || !utf8.ValidString(rendered) {
		return "", errBinding
	}
	return rendered, nil
}
