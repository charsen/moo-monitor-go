package gormcapture_test

import (
	"context"
	"errors"
	"testing"

	observer "gitee.com/charsen/moo-go-monitor/adapters/gorm"
	"gorm.io/gorm"
)

type identityInstance struct{ db *gorm.DB }

func (i identityInstance) Instance() *gorm.DB { return i.db }

type panicIdentity struct{}

func (panicIdentity) Instance() *gorm.DB { panic("private fixture capability error") }

func TestActualGORMGenerationIdentity(t *testing.T) {
	store, query, db, _, clock := setup(t, "identity-first", 0)
	first, err := observer.InstanceIdentity(query)
	if err != nil || first == (observer.Identity{}) {
		t.Fatal("missing identity", err)
	}
	clone := db.Session(&gorm.Session{NewDB: true})
	if clone == db {
		t.Fatal("fixture is not a distinct session")
	}
	for _, candidate := range []any{identityInstance{clone}, query.WithContext(context.WithValue(context.Background(), struct{}{}, "owned"))} {
		same, err := observer.InstanceIdentity(candidate)
		if err != nil || same != first {
			t.Fatal("session clone falsely became new generation", err)
		}
	}
	_, other, _, _, _ := setup(t, "identity-next", 0)
	next, err := observer.InstanceIdentity(other)
	if err != nil || next == first {
		t.Fatal("new GORM instance reused old identity", err)
	}
	owner := plugin(t, store, "identity-first", clock)
	if err = observer.Install(query, owner); err != nil {
		t.Fatal(err)
	}
	if err = observer.Install(identityInstance{clone}, owner); err != nil {
		t.Fatal("same actual owner not idempotent", err)
	}
	competing := plugin(t, store, "identity-first", clock)
	if err = observer.Install(identityInstance{clone}, competing); !errors.Is(err, observer.ErrInstalled) {
		t.Fatal("foreign owner overwritten", err)
	}
	for _, bad := range []any{nil, identityInstance{}, panicIdentity{}} {
		if _, err := observer.InstanceIdentity(bad); !errors.Is(err, observer.ErrUnsupported) {
			t.Fatal("capability failure leaked", err)
		}
	}
}
