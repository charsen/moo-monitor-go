package gormcapture

import (
	"database/sql/driver"
	"math"
	"testing"
)

type noDiagnosticMethods struct{ called *bool }

func (v noDiagnosticMethods) Value() (driver.Value, error) {
	*v.called = true
	return "fake-token", nil
}
func (v noDiagnosticMethods) String() string { *v.called = true; return "fake-token" }
func TestBindingBoundary(t *testing.T) {
	called := false
	for _, value := range []any{noDiagnosticMethods{&called}, string([]byte{255}), []byte{255}, math.NaN(), math.Inf(1), uint64(1)} {
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
