package goravel_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	adapter "gitee.com/charsen/moo-monitor-go/adapters/goravel"
	contractshttp "github.com/goravel/framework/contracts/http"
)

func TestHTTPPayloadSnapshots(t *testing.T) {
	for _, mode := range []string{"json", "form", "multipart", "get"} {
		t.Run(mode, func(t *testing.T) {
			collector, store := setup(t)
			middleware, err := adapter.NewHTTP(collector, adapter.HTTPConfig{Payload: &adapter.PayloadConfig{MaskKeys: []string{"token", "password"}}})
			if err != nil {
				t.Fatal(err)
			}
			driver, _ := adapter.NewDriver(collector, &delegate{}, nil)
			route := httpRoute(t, middleware)
			var held context.Context
			event := errors.New("fixed payload event")
			handler := func(ctx contractshttp.Context) contractshttp.Response {
				held = ctx.Context()
				if mode == "json" {
					body, err := io.ReadAll(ctx.Request().Origin().Body)
					if err != nil || string(body) != `{"name":"before","password":"fixed-body-secret","token":"fixed-token-secret"}` {
						t.Fatal("JSON stream changed", err)
					}
				}
				if mode != "get" && ctx.Request().Input("name") != "before" {
					t.Fatal("original parsed input changed")
				}
				if mode == "multipart" {
					file, _, err := ctx.Request().Origin().FormFile("upload")
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(file)
					file.Close()
					if err != nil || string(data) != "fixed-file-content-never-capture" {
						t.Fatal("business file consumed", err)
					}
				}
				eventCtx, err := middleware.EventContext(held, "one")
				if err != nil {
					t.Fatal(err)
				}
				writer(t, eventCtx, driver).With(map[string]any{adapter.ExceptionKey: event, adapter.FramesKey: fixedFrames}).Error("business event")
				if err = middleware.MarkError(held, "one", event, fixedFrames); err != nil {
					t.Fatal(err)
				}
				// These are caller-owned maps from All, never the lifecycle's already masked copy.
				ctx.Request().All()["name"] = "after"
				return ctx.Response().String(503, "original response")
			}
			if mode == "get" {
				route.Get("/event/{id}", handler)
			} else {
				route.Post("/event/{id}", handler)
			}
			var body bytes.Buffer
			method, contentType := "POST", "application/json"
			switch mode {
			case "json":
				body.WriteString(`{"name":"before","password":"fixed-body-secret","token":"fixed-token-secret"}`)
			case "form":
				contentType = "application/x-www-form-urlencoded"
				body.WriteString("name=before&password=fixed-body-secret&token=fixed-token-secret&multi=a&multi=b")
			case "multipart":
				form := multipart.NewWriter(&body)
				form.WriteField("name", "before")
				form.WriteField("password", "fixed-body-secret")
				part, err := form.CreateFormFile("upload", "fixed-filename-secret.txt")
				if err != nil {
					t.Fatal(err)
				}
				part.Write([]byte("fixed-file-content-never-capture"))
				form.Close()
				contentType = form.FormDataContentType()
			case "get":
				method = "GET"
				body.WriteString(`{"password":"fixed-body-secret"}`)
			}
			req := httptest.NewRequest(method, "http://example.test/event/route-id?token=fixed-query-secret", &body)
			req.Header.Set("Content-Type", contentType)
			response := httptest.NewRecorder()
			route.ServeHTTP(response, req)
			if req.MultipartForm != nil {
				defer req.MultipartForm.RemoveAll()
			}
			if response.Code != 503 || response.Body.String() != "original response" {
				t.Fatal("business response changed")
			}
			snapshots, err := store.Snapshot(context.Background())
			if err != nil || len(snapshots) != 1 || snapshots[0].Record.Count != 1 {
				t.Fatal("first log/5xx identity", err, len(snapshots))
			}
			encoded, err := json.Marshal(snapshots[0].Record.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "get" {
				if string(encoded) != "[]" {
					t.Fatal("GET payload captured")
				}
			} else {
				for _, value := range []string{"before", "route-id"} {
					if !bytes.Contains(encoded, []byte(value)) {
						t.Fatal("payload missing", value)
					}
				}
				if !bytes.Contains(encoded, []byte("***")) {
					t.Fatal("mask missing")
				}
			}
			for _, secret := range []string{"fixed-body-secret", "fixed-token-secret", "fixed-query-secret", "fixed-filename-secret", "fixed-file-content-never-capture"} {
				if bytes.Contains(encoded, []byte(secret)) {
					t.Fatal("secret payload retained")
				}
			}
			if mode == "multipart" && !bytes.Contains(encoded, []byte("multipart.FileHeader")) {
				t.Fatal("file marker absent")
			}
			if _, exists, err := adapter.HTTPRequestSnapshot(held); err != nil || exists {
				t.Fatal("lifecycle not closed")
			}
			if driver.Status().Failures != 0 || middleware.Status().Failures != 0 {
				t.Fatal("normal payload diagnostics")
			}
		})
	}
}

func TestHTTPPayloadRecover(t *testing.T) {
	collector, store := setup(t)
	driver, _ := adapter.NewDriver(collector, &delegate{}, nil)
	middleware, err := adapter.NewHTTP(collector, adapter.HTTPConfig{Payload: &adapter.PayloadConfig{MaskKeys: []string{"token"}}, Recover: func(ctx contractshttp.Context, value any) {
		writer(t, ctx.Context(), driver).Error("owner response log")
		if err := ctx.Response().String(502, "owner response").Abort(); err != nil {
			t.Fatal(err)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	route := httpRoute(t, middleware)
	route.Post("/panic", func(ctx contractshttp.Context) contractshttp.Response { panic(errors.New("fixed panic")) })
	response := request(route, "POST", "/panic", `{"token":"fixed-panic-secret","safe":"before"}`)
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 1 || snapshots[0].Record.Count != 1 || response.Code != 502 || response.Body.String() != "owner response" {
		t.Fatal("panic business semantics", err)
	}
	data, _ := json.Marshal(snapshots[0].Record.Payload)
	if bytes.Contains(data, []byte("fixed-panic-secret")) || !bytes.Contains(data, []byte("before")) {
		t.Fatal("panic payload copy")
	}
}
