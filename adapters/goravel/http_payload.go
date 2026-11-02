package goravel

import (
	"context"
	"mime/multipart"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"gitee.com/charsen/moo-go-monitor/mask"
	contractshttp "github.com/goravel/framework/contracts/http"
)

type PayloadConfig struct {
	ExcludePathPrefixes []string
	MaskKeys            []string
	MaxStringChars      int
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
	if len(h.payloadExcluded) > 0 {
		requestPath := ctx.Request().Path()
		if !validPayloadPath(requestPath) {
			return nil, ErrAdapter
		}
		for _, prefix := range h.payloadExcluded {
			if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
				return nil, nil
			}
		}
	}
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

func payloadPrefixes(cfg *PayloadConfig) ([]string, error) {
	if cfg == nil {
		return nil, nil
	}
	if len(cfg.ExcludePathPrefixes) > 64 {
		return nil, ErrHTTPContext
	}
	result := make([]string, 0, len(cfg.ExcludePathPrefixes))
	seen := map[string]bool{}
	for _, prefix := range cfg.ExcludePathPrefixes {
		if prefix == "/" || !validPayloadPath(prefix) || path.Clean(prefix) != prefix {
			return nil, ErrHTTPContext
		}
		if !seen[prefix] {
			result = append(result, prefix)
			seen[prefix] = true
		}
	}
	return result, nil
}
func validPayloadPath(value string) bool {
	if value == "" || len(value) > 2048 || !utf8.ValidString(value) || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\%?#") {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return value == "/" || path.Clean(value) == strings.TrimSuffix(value, "/")
}
