package gormcapture

import (
	"regexp"
	"strconv"
	"unicode/utf8"

	"gitee.com/charsen/moo-go-monitor/slowquery"
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"
)

var postgresPlaceholder = regexp.MustCompile(`\$([0-9]+)`)

func supportedDialect(name string) bool {
	return name == "sqlite" || name == "postgres" || name == "mysql" || name == "sqlserver"
}

// postgresRendered generates display text, never executable SQL. The locked
// dialect's Explain replaces numeric placeholders even in quoted text.
func postgresRendered(dialect gorm.Dialector, raw string, values []any) (string, error) {
	limit := slowquery.MaxRenderedSQLBytes
	if len(raw) > limit || !utf8.ValidString(raw) || len(values) > limit/16 {
		return "", errBinding
	}
	if len(values) > 0 {
		if mode, ok := values[0].(pgx.QueryExecMode); ok {
			switch mode {
			case pgx.QueryExecModeCacheStatement, pgx.QueryExecModeCacheDescribe, pgx.QueryExecModeDescribeExec, pgx.QueryExecModeExec, pgx.QueryExecModeSimpleProtocol:
				values = values[1:]
			default:
				return "", errBinding
			}
		}
	}
	normalized, err := bindings(values)
	if err != nil {
		return "", err
	}
	// Bound repeated-placeholder expansion before calling Explain. Use conservative
	// sizes (including doubled string quotes); arithmetic is checked against remaining budget.
	costs := make([]int, len(normalized))
	for i, value := range normalized {
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
		costs[i] = cost
	}
	remaining := limit - len(raw)
	offset := 0
	for offset < len(raw) {
		match := postgresPlaceholder.FindStringSubmatchIndex(raw[offset:])
		if match == nil {
			break
		}
		cost := 2 // unmatched placeholders gain delimiters in the mature formatter
		index, err := strconv.Atoi(raw[offset+match[2] : offset+match[3]])
		if err == nil && index > 0 && index <= len(costs) {
			cost = costs[index-1]
		}
		if cost > remaining {
			return "", errBinding
		}
		remaining -= cost
		offset += match[1]
	}
	rendered := dialect.Explain(raw, normalized...)
	if rendered == "" || len(rendered) > limit || !utf8.ValidString(rendered) {
		return "", errBinding
	}
	return rendered, nil
}
