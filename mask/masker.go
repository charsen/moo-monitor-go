// Package mask implements the bounded basic SensitiveMasker source contract.
package mask

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	authorization = regexp.MustCompile(`(\bauthorization[\t\n\f\r\v ]*[:=][\t\n\f\r\v ]*)(?:bearer|basic|digest)?[\t\n\f\r\v ]*[^\t\n\f\r\v "',&;]+`)
	jwt           = regexp.MustCompile(`eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`)
	bearer        = regexp.MustCompile(`\bbearer[\t\n\f\r\v ]+[a-z0-9._\-]+`)
)

// Masker owns a copy of its keys. Its configuration and compiled patterns are immutable.
type Masker struct {
	keys       []string
	keySecrets *regexp.Regexp
}

// New accepts valid UTF-8 configuration; PHP's wider byte-pattern inputs are not supported.
func New(keys []string) (*Masker, error) {
	m := &Masker{}
	alternatives := make([]string, 0, len(keys))
	for _, key := range keys {
		if !utf8.ValidString(key) {
			return nil, fmt.Errorf("mask key must be valid UTF-8")
		}
		// PHP array_filter removes both empty strings and the string "0".
		if key == "" || key == "0" {
			continue
		}
		key = lowerASCII(key)
		m.keys = append(m.keys, key)
		alternatives = append(alternatives, regexp.QuoteMeta(key))
	}
	if len(alternatives) > 0 {
		pattern := `([a-z0-9_.\-]*(?:` + strings.Join(alternatives, "|") + `)[a-z0-9_.\-]*[\t\n\f\r\v ]*["']?[\t\n\f\r\v ]*[:=][\t\n\f\r\v ]*["']?)([^\t\n\f\r\v "',&;]+)`
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("compile sensitive key pattern: %w", err)
		}
		m.keySecrets = compiled
	}
	return m, nil
}

func (m *Masker) ShouldMaskKey(key string) bool {
	key = lowerASCII(key)
	for _, needle := range m.keys {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

// MaskURL preserves raw query order/encoding and only masks matching values after '='.
// Userinfo, paths and fragments are outside this source method's masking scope.
func (m *Masker) MaskURL(value string) string {
	question := strings.IndexByte(value, '?')
	if question < 0 {
		return value
	}
	prefix, query := value[:question], value[question+1:]
	fragment := ""
	if hash := strings.IndexByte(query, '#'); hash >= 0 {
		fragment, query = query[hash:], query[:hash]
	}
	if query == "" {
		return value
	}
	parts := make([]string, 0)
	for _, pair := range strings.Split(query, "&") {
		if pair == "" {
			continue
		}
		key, _, hasEqual := strings.Cut(pair, "=")
		if hasEqual && m.ShouldMaskKey(decodeQueryKey(key)) {
			pair = key + "=***"
		}
		parts = append(parts, pair)
	}
	return prefix + "?" + strings.Join(parts, "&") + fragment
}

// MaskSecrets keeps the source order: Authorization, JWT, Bearer, then configured keys.
// This is a finite text-pattern matcher, not a guarantee that arbitrary text is safe.
func (m *Masker) MaskSecrets(value string) string {
	value = replaceLowerASCII(value, authorization, "***", true)
	value = jwt.ReplaceAllString(value, "***JWT***")
	value = replaceLowerASCII(value, bearer, "Bearer ***", false)
	if m.keySecrets != nil {
		value = replaceLowerASCII(value, m.keySecrets, "***", true)
	}
	return value
}

// Match ASCII-folded bytes but retain original captured prefixes and untouched text.
// Go's (?i) also folds Unicode characters, unlike the source's byte-oriented rules.
func replaceLowerASCII(value string, pattern *regexp.Regexp, replacement string, keepPrefix bool) string {
	matches := pattern.FindAllStringSubmatchIndex(lowerASCII(value), -1)
	if len(matches) == 0 {
		return value
	}
	var result strings.Builder
	last := 0
	for _, match := range matches {
		result.WriteString(value[last:match[0]])
		if keepPrefix {
			result.WriteString(value[match[2]:match[3]])
		}
		result.WriteString(replacement)
		last = match[1]
	}
	result.WriteString(value[last:])
	return result.String()
}

func lowerASCII(value string) string {
	bytes := []byte(value)
	for i, b := range bytes {
		if b >= 'A' && b <= 'Z' {
			bytes[i] = b + ('a' - 'A')
		}
	}
	return string(bytes)
}

// PHP urldecode accepts malformed percent sequences and converts '+' to space.
func decodeQueryKey(value string) string {
	hex := func(b byte) (byte, bool) {
		switch {
		case b >= '0' && b <= '9':
			return b - '0', true
		case b >= 'a' && b <= 'f':
			return b - 'a' + 10, true
		case b >= 'A' && b <= 'F':
			return b - 'A' + 10, true
		}
		return 0, false
	}
	var decoded strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '+' {
			decoded.WriteByte(' ')
			continue
		}
		if value[i] == '%' && i+2 < len(value) {
			high, validHigh := hex(value[i+1])
			low, validLow := hex(value[i+2])
			if validHigh && validLow {
				decoded.WriteByte(high<<4 | low)
				i += 2
				continue
			}
		}
		decoded.WriteByte(value[i])
	}
	return decoded.String()
}
