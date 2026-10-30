package slowquery

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// fillBindings preserves the source's sequential '?' algorithm, including its quoted-'?' limitation.
func fillBindings(raw string, bindings []any) (string, error) {
	if !utf8.ValidString(raw) {
		return "", ErrInput
	}
	parts := strings.Split(raw, "?")
	var output strings.Builder
	output.WriteString(parts[0])
	for index, part := range parts[1:] {
		text := "?"
		if index < len(bindings) {
			var err error
			text, err = bindingText(bindings[index])
			if err != nil {
				return "", err
			}
		}
		output.WriteString(text)
		output.WriteString(part)
	}
	return output.String(), nil
}
func bindingText(value any) (string, error) {
	switch item := value.(type) {
	case nil:
		return "NULL", nil
	case bool:
		if item {
			return "1", nil
		}
		return "0", nil
	case int:
		return strconv.Itoa(item), nil
	case int64:
		return strconv.FormatInt(item, 10), nil
	case float64:
		if math.IsNaN(item) || math.IsInf(item, 0) {
			return "", ErrInput
		}
		text := strconv.FormatFloat(item, 'G', 14, 64)
		if mantissa, exponent, ok := strings.Cut(text, "E"); ok {
			if !strings.Contains(mantissa, ".") {
				mantissa += ".0"
			}
			sign := ""
			if strings.HasPrefix(exponent, "+") || strings.HasPrefix(exponent, "-") {
				sign = exponent[:1]
				exponent = exponent[1:]
			}
			exponent = strings.TrimLeft(exponent, "0")
			if exponent == "" {
				exponent = "0"
			}
			text = mantissa + "E" + sign + exponent
		}
		return text, nil
	case string:
		if !utf8.ValidString(item) {
			return "", ErrInput
		}
		return "'" + strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(item) + "'", nil
	case time.Time:
		return "'" + item.Format("2006-01-02 15:04:05") + "'", nil
	default:
		return "", ErrInput // Never invoke arbitrary Stringer/driver/object behavior.
	}
}

// roundMillis implements the positive, two-decimal PHP 8.3 rounding path, not a general PHP runtime.
// The source's 14-digit pre-round is needed for values immediately below a decimal half.
// https://github.com/php/php-src/blob/php-8.3.35/ext/standard/math.c#L116
func roundMillis(value float64) float64 {
	if value == 0 {
		return value
	}
	exponent := int(math.Floor(math.Log10(value)))
	if value < math.Pow10(exponent) {
		exponent--
	} else if value >= math.Pow10(exponent+1) {
		exponent++
	}
	precision := 14 - exponent
	scaled := value * 100
	if precision > 2 && precision-15 < 2 {
		intermediate := float64(value * math.Pow10(precision))
		scaled = math.Floor(intermediate+0.5) / math.Pow10(precision-2)
	} else if scaled >= 1e15 {
		return value
	}
	return math.Floor(scaled+0.5) / 100
}
