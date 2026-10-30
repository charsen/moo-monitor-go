package gormcapture

import (
	"errors"
	"math"
	"time"
	"unicode/utf8"
)

var errBinding = errors.New("unsupported GORM monitor binding")

// Only immutable scalar values and explicit known pointer forms are copied.
// driver.Valuer/Stringer and arbitrary objects are never invoked to collect diagnostics.
func bindings(values []any) ([]any, error) {
	result := make([]any, 0, len(values))
	for _, value := range values {
		switch item := value.(type) {
		case nil, bool, int, int64, time.Time:
			result = append(result, item)
		case string:
			if !utf8.ValidString(item) {
				return nil, errBinding
			}
			result = append(result, item)
		case float64:
			if math.IsNaN(item) || math.IsInf(item, 0) {
				return nil, errBinding
			}
			result = append(result, item)
		case []byte:
			if !utf8.Valid(item) {
				return nil, errBinding
			}
			result = append(result, string(item))
		case *string:
			if item == nil {
				result = append(result, nil)
			} else if utf8.ValidString(*item) {
				result = append(result, *item)
			} else {
				return nil, errBinding
			}
		case *int:
			if item == nil {
				result = append(result, nil)
			} else {
				result = append(result, *item)
			}
		case *int64:
			if item == nil {
				result = append(result, nil)
			} else {
				result = append(result, *item)
			}
		case *bool:
			if item == nil {
				result = append(result, nil)
			} else {
				result = append(result, *item)
			}
		case *float64:
			if item == nil {
				result = append(result, nil)
			} else if !math.IsNaN(*item) && !math.IsInf(*item, 0) {
				result = append(result, *item)
			} else {
				return nil, errBinding
			}
		case *time.Time:
			if item == nil {
				result = append(result, nil)
			} else {
				result = append(result, *item)
			}
		default:
			return nil, errBinding
		}
	}
	return result, nil
}
