package gormcapture

import "gorm.io/gorm"

// Identity is an opaque callback-registry identity, not a *gorm.DB session
// pointer. Public Session/WithContext clones share it; a new Open has a new one.
// Its private value cannot be used to mutate the registry or serialized as data.
type Identity struct{ callbackSet any }

// InstanceIdentity inspects only public capabilities. Like Install, callers must
// use a quiescent connection publication window; it does not make external GORM
// plugin registration or concurrent Fresh safe.
func InstanceIdentity(query any) (Identity, error) {
	db, err := queryInstance(query)
	if err != nil {
		return Identity{}, err
	}
	return Identity{callbackSet: db.Callback()}, nil
}

func queryInstance(query any) (db *gorm.DB, failure error) {
	defer func() {
		if recover() != nil {
			db = nil
			failure = ErrUnsupported
		}
	}()
	capable, ok := query.(interface{ Instance() *gorm.DB })
	if !ok {
		return nil, ErrUnsupported
	}
	db = capable.Instance()
	if db == nil || db.Config == nil || db.Dialector == nil || !supportedDialect(db.Dialector.Name()) || db.Callback() == nil {
		return nil, ErrUnsupported
	}
	return db, nil
}
