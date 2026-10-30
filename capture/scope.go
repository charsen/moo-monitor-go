package capture

import (
	"context"
	"errors"
	"unicode/utf8"
)

var (
	ErrScopeClosed    = errors.New("capture event scope is closed")
	ErrScopeFull      = errors.New("capture event scope is full")
	ErrEventIdentity  = errors.New("invalid capture event identity")
	ErrEventUncertain = errors.New("capture event outcome is unknown; identity cannot be counted again")
)

type scopeEntry struct {
	hash      string
	rank      int
	uncertain bool
}

// EventScope is owned by one bounded request/job lifecycle, never a global error-object cache.
type EventScope struct {
	gate    chan struct{}
	entries map[string]scopeEntry
	max     int
	closed  bool
}

func NewEventScope(maxIDs int) (*EventScope, error) {
	if maxIDs < 1 || maxIDs > 4096 {
		return nil, ErrEventIdentity
	}
	s := &EventScope{gate: make(chan struct{}, 1), entries: map[string]scopeEntry{}, max: maxIDs}
	s.gate <- struct{}{}
	return s, nil
}
func (s *EventScope) Close() {
	<-s.gate
	defer func() { s.gate <- struct{}{} }()
	clear(s.entries)
	s.closed = true
}
func (s *EventScope) enter(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.gate:
		if err := ctx.Err(); err != nil {
			s.leave()
			return err
		}
		if s.closed {
			s.leave()
			return ErrScopeClosed
		}
		return nil
	}
}
func (s *EventScope) leave() { s.gate <- struct{}{} }

type bindingKey struct{}
type eventBinding struct {
	scope *EventScope
	id    string
}
type internalKey struct{}

func WithEvent(ctx context.Context, scope *EventScope, id string) context.Context {
	return context.WithValue(ctx, bindingKey{}, eventBinding{scope, id})
}
func EventFromContext(ctx context.Context) (*EventScope, string) {
	if ctx == nil {
		return nil, ""
	}
	binding, _ := ctx.Value(bindingKey{}).(eventBinding)
	return binding.scope, binding.id
}
func InternalContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, internalKey{}, true)
}
func IsInternal(ctx context.Context) bool { return ctx != nil && ctx.Value(internalKey{}) == true }
func validID(id string) bool              { return len(id) > 0 && len(id) <= 128 && utf8.ValidString(id) }
