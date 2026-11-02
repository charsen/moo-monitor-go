package goravel

import (
	"context"
	"mime/multipart"

	"gitee.com/charsen/moo-go-monitor/mask"
	contractshttp "github.com/goravel/framework/contracts/http"
)

type PayloadConfig struct {
	MaskKeys       []string
	MaxStringChars int
}

// capturePayload uses only the framework's already parsed All, never Body or a file handle.
func (h *HTTP) capturePayload(ctx contractshttp.Context, method string) (payload any, failure error) {
	if h.payloadMasker == nil {
		return nil, nil
	}
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return nil, nil
	}
	defer func() {
		if recover() != nil {
			payload = nil
			failure = ErrAdapter
		}
	}()
	all := ctx.Request().All()
	remaining := 5000
	if len(all) > remaining {
		return nil, ErrAdapter
	}
	remaining -= len(all)
	normalized := make(map[string]any, len(all))
	for key, value := range all {
		switch values := value.(type) {
		case []string:
			if len(values) > remaining {
				return nil, ErrAdapter
			}
			remaining -= len(values)
			items := make([]any, len(values))
			for i, v := range values {
				items[i] = v
			}
			normalized[key] = items
		case *multipart.FileHeader:
			normalized[key] = mask.OpaqueObject{Name: "multipart.FileHeader"}
		case []*multipart.FileHeader:
			if len(values) > remaining {
				return nil, ErrAdapter
			}
			remaining -= len(values)
			items := make([]any, len(values))
			for i := range values {
				items[i] = mask.OpaqueObject{Name: "multipart.FileHeader"}
			}
			normalized[key] = items
		default:
			normalized[key] = value
		}
	}
	result, err := h.payloadMasker.MaskPayload(normalized, h.payloadLimit)
	if err != nil {
		return nil, ErrAdapter
	}
	return result, nil
}
func copiedHTTPPayload(ctx context.Context) any {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(httpKey{}).(*httpState)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return nil
	}
	return state.payload
}
func payloadMasker(config *PayloadConfig) (*mask.Masker, int, error) {
	if config == nil {
		return nil, 0, nil
	}
	limit := config.MaxStringChars
	if limit == 0 {
		limit = 200
	}
	if limit < 0 {
		return nil, 0, ErrHTTPContext
	}
	masker, err := mask.New(config.MaskKeys)
	if err != nil {
		return nil, 0, ErrHTTPContext
	}
	return masker, limit, nil
}
