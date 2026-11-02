package goravel

import (
	"encoding/json"
	"strings"
	"testing"

	httpmocks "github.com/goravel/framework/mocks/http"
)

type payloadTrap struct{}

func (payloadTrap) String() string { panic("payload object must not stringify") }
func TestPayloadBoundaryFailuresAndBudget(t *testing.T) {
	masker, limit, err := payloadMasker(&PayloadConfig{MaskKeys: []string{"token"}})
	if err != nil {
		t.Fatal(err)
	}
	h := &HTTP{payloadMasker: masker, payloadLimit: limit}
	if value, err := h.capturePayload(nil, "GET"); value != nil || err != nil {
		t.Fatal("GET read request")
	}
	for _, unknown := range []any{payloadTrap{}, int(1)} {
		ctx := httpmocks.NewContext(t)
		request := httpmocks.NewContextRequest(t)
		ctx.On("Request").Return(request)
		request.On("All").Return(map[string]any{"unknown": unknown})
		if value, err := h.capturePayload(ctx, "POST"); err == nil || value != nil {
			t.Fatal("unknown object retained")
		}
	}
	ctx := httpmocks.NewContext(t)
	request := httpmocks.NewContextRequest(t)
	ctx.On("Request").Return(request)
	all := map[string]any{}
	for i := 0; i < 5005; i++ {
		all[strings.Repeat("a", i%20)+string(rune(0x1000+i))] = "fixed"
	}
	request.On("All").Return(all)
	value, err := h.capturePayload(ctx, "POST")
	if err == nil || value != nil {
		t.Fatal("root normalization exceeded budget")
	}
	nestedCtx := httpmocks.NewContext(t)
	nestedRequest := httpmocks.NewContextRequest(t)
	nestedCtx.On("Request").Return(nestedRequest)
	nestedRequest.On("All").Return(map[string]any{"nested": all})
	value, err = h.capturePayload(nestedCtx, "POST")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil || !strings.Contains(string(data), "__truncated__") {
		t.Fatal("budget not visible", err)
	}
	if _, _, err := payloadMasker(&PayloadConfig{MaxStringChars: -1}); err == nil {
		t.Fatal("negative cap")
	}
}

func TestPayloadRepeatedValuesBudget(t *testing.T) {
	masker, limit, _ := payloadMasker(&PayloadConfig{})
	h := &HTTP{payloadMasker: masker, payloadLimit: limit}
	ctx := httpmocks.NewContext(t)
	request := httpmocks.NewContextRequest(t)
	ctx.On("Request").Return(request)
	request.On("All").Return(map[string]any{"repeated": make([]string, 5001)})
	if value, err := h.capturePayload(ctx, "POST"); err == nil || value != nil {
		t.Fatal("copied over-budget repeated values")
	}
}
