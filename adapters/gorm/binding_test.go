package gormcapture

import (
	"database/sql/driver"
	"github.com/goravel/framework/support/carbon"
	"gorm.io/gorm"
	"math"
	"testing"
	"time"
)

type noDiagnosticMethods struct{ called *bool }

func (v noDiagnosticMethods) Value() (driver.Value, error) {
	*v.called = true
	return "fake-token", nil
}
func (v noDiagnosticMethods) String() string { *v.called = true; return "fake-token" }
func TestBindingBoundary(t *testing.T) {
	called := false
	for _, value := range []any{noDiagnosticMethods{&called}, string([]byte{255}), []byte{255}, math.NaN(), math.Inf(1), ^uint64(0)} {
		if safe, err := bindings([]any{value}); err == nil || safe != nil {
			t.Fatal("unsupported binding accepted")
		}
	}
	if called {
		t.Fatal("collection invoked arbitrary user method")
	}
	text := "value"
	var absent *int64
	safe, err := bindings([]any{&text, absent, []byte("valid"), int64(0), false})
	if err != nil || len(safe) != 5 || safe[0] != "value" || safe[1] != nil || safe[2] != "valid" {
		t.Fatal("known scalar copy failed", err)
	}
}

func TestNativeNumericWidths(t *testing.T) {
	values := []any{int8(0), int8(-128), int16(-32768), int32(-2147483648), uint8(255), uint16(65535), uint32(4294967295), uint(0), uint64(math.MaxInt64), float32(1.5)}
	safe, err := bindings(values)
	if err != nil || len(safe) != len(values) {
		t.Fatal("native widths rejected", err)
	}
	expected := []any{int64(0), int64(-128), int64(-32768), int64(-2147483648), int64(255), int64(65535), int64(4294967295), int64(0), int64(math.MaxInt64), float64(1.5)}
	for i, value := range expected {
		if safe[i] != value {
			t.Fatal("numeric value changed", i)
		}
	}
	for _, value := range []any{uint64(math.MaxInt64) + 1, float32(math.Inf(1)), float32(math.NaN())} {
		if safe, err := bindings([]any{value}); err == nil || safe != nil {
			t.Fatal("invalid numeric accepted")
		}
	}
}

func TestFrameworkDateBindings(t *testing.T) {
	instant := time.Date(2026, 10, 30, 12, 34, 56, 123000000, time.FixedZone("Shanghai", 8*3600))
	date := carbon.NewDateTime(carbon.New(instant))
	var absent *carbon.DateTime
	deleted := gorm.DeletedAt{Time: instant, Valid: true}
	var absentDeleted *gorm.DeletedAt
	safe, err := bindings([]any{date, *date, absent, carbon.DateTime{}, carbon.NewDateTime(carbon.ZeroValue()), deleted, &deleted, absentDeleted, gorm.DeletedAt{Time: instant}})
	if err != nil || len(safe) != 9 {
		t.Fatal("known dates rejected", err)
	}
	for _, index := range []int{0, 1, 5, 6} {
		value, ok := safe[index].(time.Time)
		if !ok || !value.Equal(instant) || value.Format(time.RFC3339Nano) != instant.Format(time.RFC3339Nano) {
			t.Fatal("date timezone changed", index)
		}
	}
	for _, index := range []int{2, 3, 4, 7, 8} {
		if safe[index] != nil {
			t.Fatal("nullable date changed", index)
		}
	}
	bad := carbon.NewDateTime(carbon.Parse("fixed-invalid-date"))
	if safe, err := bindings([]any{bad}); err == nil || safe != nil {
		t.Fatal("invalid framework date accepted")
	}
}
