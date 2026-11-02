package goravel

import (
	"encoding/json"
	"strings"
	"testing"

	httpmocks "github.com/goravel/framework/mocks/http"
)

func TestPayloadExclusionPrecedesAll(t *testing.T) {
	input := []string{"/scaffold"}
	prefixes, err := payloadPrefixes(&PayloadConfig{ExcludePathPrefixes: input})
	if err != nil {
		t.Fatal(err)
	}
	input[0] = "/changed"
	masker, limit, _ := payloadMasker(&PayloadConfig{})
	h := &HTTP{payloadMasker: masker, payloadLimit: limit, payloadExcluded: prefixes}
	for _, value := range []string{"/scaffold", "/scaffold/", "/scaffold/login", "/scaffold/settings/ai"} {
		ctx := httpmocks.NewContext(t)
		request := httpmocks.NewContextRequest(t)
		ctx.On("Request").Return(request)
		request.On("Path").Return(value)
		if data, err := h.capturePayload(ctx, "POST"); data != nil || err != nil {
			t.Fatal("excluded path read payload or lost copied prefix", value, err)
		}
		request.AssertNotCalled(t, "All")
	}
	ctx := httpmocks.NewContext(t)
	request := httpmocks.NewContextRequest(t)
	ctx.On("Request").Return(request)
	request.On("Path").Return("/scaffold-evil/control")
	request.On("All").Return(map[string]any{"value": "ordinary-control"})
	data, err := h.capturePayload(ctx, "POST")
	encoded, _ := json.Marshal(data)
	if err != nil || !strings.Contains(string(encoded), "ordinary-control") {
		t.Fatal("path segment boundary overexcluded", err)
	}
	for _, mode := range []string{"panic", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			ctx := httpmocks.NewContext(t)
			request := httpmocks.NewContextRequest(t)
			ctx.On("Request").Return(request)
			if mode == "panic" {
				request.On("Path").Panic("fixed path getter")
			} else {
				request.On("Path").Return("//scaffold/login")
			}
			if data, err := h.capturePayload(ctx, "POST"); data != nil || err == nil {
				t.Fatal("path failure fell through to All")
			}
			request.AssertNotCalled(t, "All")
		})
	}
	for _, prefix := range []string{"", "/", "scaffold", "//scaffold", "/scaffold/", "/scaffold/../api", "/scaffold%2f", "/scaffold?query", "/scaffold\\other", "/scaffold\n"} {
		if _, err := payloadPrefixes(&PayloadConfig{ExcludePathPrefixes: []string{prefix}}); err == nil {
			t.Fatal("invalid prefix accepted", prefix)
		}
	}
}
