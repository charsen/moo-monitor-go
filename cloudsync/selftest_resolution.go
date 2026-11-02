package cloudsync

import (
	"context"
	"errors"
)

type SelfTestOptions struct{ ResolveRuntime bool }
type SelfTestResolution struct {
	Applicable, Attempted, Resolved bool
	Hash, Outcome                   string
}

// RuntimeResolver consumes the existing strict resolution protocol, not a new token source.
type RuntimeResolver interface {
	ResolveRuntime(context.Context, string, *string, *string) (MCPObject, error)
}

func resolveSelfTestRuntime(ctx context.Context, resolver RuntimeResolver, hash string) (result SelfTestResolution) {
	result = SelfTestResolution{Applicable: true, Hash: hash, Outcome: "unavailable"}
	if resolver == nil {
		return result
	}
	result.Attempted = true
	defer func() {
		if recover() != nil {
			result.Resolved = false
			result.Outcome = "resolver_panic"
		}
	}()
	note, by := "moo:cloud:test 自检记录", "moo:cloud:test"
	receipt, err := resolver.ResolveRuntime(ctx, hash, &note, &by)
	if err != nil {
		result.Outcome = selfTestResolutionFailure(err)
		return result
	}
	record, err := mcpDetail(receipt, "runtime", "hash", hash)
	if err != nil || rawString(record["status"]) != "resolved" || !mcpFlag(receipt, "already_resolved") {
		result.Outcome = "invalid_receipt"
		return result
	}
	result.Resolved = true
	result.Outcome = "resolved"
	return result
}
func selfTestResolutionFailure(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrHTTPReceipt):
		return "invalid_receipt"
	case errors.Is(err, ErrResponseTooLarge):
		return "response_too_large"
	}
	var failure *HTTPError
	if errors.As(err, &failure) && failure != nil && (failure.Status == 401 || failure.Status == 403) {
		return "permission_denied"
	}
	if errors.Is(err, ErrHTTPStatus) {
		return "http_status"
	}
	if errors.Is(err, ErrHTTPTransport) {
		return "transport"
	}
	return "failed"
}
