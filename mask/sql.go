package mask

import (
	"regexp"
	"strings"
)

const (
	sqlSpace  = `[\t\n\f\r\v ]`
	sqlQuoted = `'(?:[^'\\]|\\.)*'|"(?:[^"\\]|\\.)*"`
	sqlTuple  = `\((?:` + sqlQuoted + `|[^()])*\)`
)

var (
	insertValues = regexp.MustCompile(`(\binsert` + sqlSpace + `+into\b[^(]*\()([^)]*)(\)` + sqlSpace + `*values` + sqlSpace + `*)((?:` + sqlSpace + `*` + sqlTuple + sqlSpace + `*,?)*)`)
	valueTuple   = regexp.MustCompile(sqlTuple)
)

// MaskSensitiveSQL follows the source comparison-then-INSERT pattern algorithm.
// It neither executes SQL nor implicitly applies MaskSecrets; unsupported syntax may remain.
func (m *Masker) MaskSensitiveSQL(value string) string {
	if len(m.keys) == 0 || value == "" {
		return value
	}
	value = replaceLowerASCII(value, m.sqlValues, "***", true)
	matches := insertValues.FindAllStringSubmatchIndex(lowerASCII(value), -1)
	if len(matches) == 0 {
		return value
	}
	var result strings.Builder
	last := 0
	for _, match := range matches {
		result.WriteString(value[last:match[0]])
		columns := splitSQLList(value[match[4]:match[5]])
		maskIndexes := make(map[int]bool)
		for index, column := range columns {
			if m.ShouldMaskKey(cleanSQLIdentifier(column)) {
				maskIndexes[index] = true
			}
		}
		if len(maskIndexes) == 0 {
			result.WriteString(value[match[0]:match[1]])
		} else {
			result.WriteString(value[match[2]:match[3]])
			result.WriteString(value[match[4]:match[5]])
			result.WriteString(value[match[6]:match[7]])
			tuples := value[match[8]:match[9]]
			result.WriteString(valueTuple.ReplaceAllStringFunc(tuples, func(tuple string) string {
				values := splitSQLList(tuple[1 : len(tuple)-1])
				for index := range values {
					if maskIndexes[index] {
						values[index] = "***"
					}
				}
				return "(" + strings.Join(values, ", ") + ")"
			}))
		}
		last = match[1]
	}
	result.WriteString(value[last:])
	return result.String()
}

// Preserve the source quote/backslash state machine, including its finite grammar.
func splitSQLList(value string) []string {
	items := make([]string, 0)
	var current strings.Builder
	var quote byte
	escaped := false
	for i := 0; i < len(value); i++ {
		b := value[i]
		if quote != 0 {
			current.WriteByte(b)
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == quote {
				quote = 0
			}
			continue
		}
		if b == '\'' || b == '"' {
			quote = b
			current.WriteByte(b)
			continue
		}
		if b == ',' {
			items = append(items, trimSQLPart(current.String()))
			current.Reset()
			continue
		}
		current.WriteByte(b)
	}
	if tail := trimSQLPart(current.String()); tail != "" || value != "" {
		items = append(items, tail)
	}
	return items
}

func trimSQLPart(value string) string {
	return strings.Trim(value, " \t\n\r\x00\x0b")
}

func cleanSQLIdentifier(value string) string {
	value = trimSQLPart(value)
	if dot := strings.LastIndexByte(value, '.'); dot >= 0 {
		value = value[dot+1:]
	}
	return strings.Trim(value, " \t\n\r\x00\x0b`\"[]")
}
