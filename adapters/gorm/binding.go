package gormcapture

import (
	"errors"
	"github.com/goravel/framework/support/carbon"
	"gorm.io/gorm"
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
		case int8:
			result = append(result, int64(item))
		case int16:
			result = append(result, int64(item))
		case int32:
			result = append(result, int64(item))
		case uint8:
			result = append(result, int64(item))
		case uint16:
			result = append(result, int64(item))
		case uint32:
			result = append(result, int64(item))
		case uint:
			if uint64(item) > math.MaxInt64 {
				return nil, errBinding
			}
			result = append(result, int64(item))
		case uint64:
			if item > math.MaxInt64 {
				return nil, errBinding
			}
			result = append(result, int64(item))
		case float32:
			converted := float64(item)
			if math.IsNaN(converted) || math.IsInf(converted, 0) {
				return nil, errBinding
			}
			result = append(result, converted)
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
		case carbon.DateTime:
			converted, err := carbonBinding(item.Carbon)
			if err != nil {
				return nil, err
			}
			result = append(result, converted)
		case *carbon.DateTime:
			if item == nil {
				result = append(result, nil)
				break
			}
			converted, err := carbonBinding(item.Carbon)
			if err != nil {
				return nil, err
			}
			result = append(result, converted)
		case gorm.DeletedAt:
			if item.Valid {
				result = append(result, item.Time)
			} else {
				result = append(result, nil)
			}
		case *gorm.DeletedAt:
			if item != nil && item.Valid {
				result = append(result, item.Time)
			} else {
				result = append(result, nil)
			}
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

// These are locked framework dates, not an arbitrary Valuer or Stringer capability.
func carbonBinding(value *carbon.Carbon) (any, error) {
	if value == nil {
		return nil, nil
	}
	if value.HasError() {
		return nil, errBinding
	}
	if value.IsZero() || value.IsEmpty() {
		return nil, nil
	}
	return value.StdTime(), nil
}
