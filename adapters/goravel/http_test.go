package goravel_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	adapter "gitee.com/charsen/moo-go-monitor/adapters/goravel"
	"gitee.com/charsen/moo-go-monitor/capture"
	contractshttp "github.com/goravel/framework/contracts/http"
	configmocks "github.com/goravel/framework/mocks/config"
	logmocks "github.com/goravel/framework/mocks/log"
	goravelgin "github.com/goravel/gin"
	"github.com/stretchr/testify/mock"
)

func httpRoute(t *testing.T, middleware *adapter.HTTP) *goravelgin.Route {
	t.Helper()
	config := configmocks.NewConfig(t)
	config.On("GetInt", "http.request_timeout", 3).Return(0)
	config.On("GetInt", "http.drivers.gin.body_limit", 4096).Return(4096)
	config.On("GetBool", "app.debug").Return(false)
	config.On("Get", "http.drivers.gin.template").Return(nil)
	route, err := goravelgin.NewRoute(config, map[string]any{"driver": "gin"})
	if err != nil {
		t.Fatal(err)
	}
	route.SetGlobalMiddleware([]contractshttp.Middleware{middleware})
	return route
}
func request(route *goravelgin.Route, method, path, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	route.ServeHTTP(response, req)
	return response
}
func TestHTTPLifecycle(t *testing.T) {
	for _, status := range []int{200, 422, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			collector, store := setup(t)
			userID := "actor"
			middleware, err := adapter.NewHTTP(collector, adapter.HTTPConfig{Identity: func(contractshttp.Context) (adapter.HTTPIdentity, error) {
				return adapter.HTTPIdentity{ID: &userID}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			route := httpRoute(t, middleware)
			var held context.Context
			var eventContext context.Context
			route.Post("/event", func(ctx contractshttp.Context) contractshttp.Response {
				held = ctx.Context()
				eventContext, err = middleware.EventContext(held, "late")
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(ctx.Request().Origin().Body)
				if err != nil || string(data) != `{"token":"fake-body"}` {
					t.Fatal("body consumed", err)
				}
				userID = "mutated after snapshot"
				return ctx.Response().String(status, "unchanged response")
			})
			response := request(route, "POST", "http://example.test/event?token=fake-query", `{"token":"fake-body"}`)
			if response.Code != status || response.Body.String() != "unchanged response" {
				t.Fatal("response changed")
			}
			snapshots, err := store.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if status < 500 {
				if len(snapshots) != 0 {
					t.Fatal("non5xx captured")
				}
			} else {
				if len(snapshots) != 1 {
					t.Fatal("missing5xx")
				}
				record := snapshots[0].Record
				if record.Exception.Class != "goravel.HTTPStatus" || record.Meta["source"] != "http_5xx" || *record.Request.UserID != "actor" || !strings.Contains(*record.Request.URL, "token=***") {
					t.Fatal("HTTP snapshot")
				}
				if payload, ok := record.Payload.([]any); !ok || len(payload) != 0 {
					t.Fatal("body payload auto-read")
				}
			}
			if _, err = middleware.EventContext(held, "late"); !errors.Is(err, adapter.ErrHTTPContext) {
				t.Fatal("lifecycle not closed")
			}
			if _, err = collector.Capture(eventContext, errors.New("late"), capture.Options{Frames: fixedFrames}); !errors.Is(err, capture.ErrScopeClosed) {
				t.Fatal("scope survived request", err)
			}
		})
	}
}
func TestHTTPExplicitEvent(t *testing.T) {
	collector, store := setup(t)
	middleware, _ := adapter.NewHTTP(collector, adapter.HTTPConfig{})
	driver, _ := adapter.NewDriver(collector, &delegate{}, nil)
	route := httpRoute(t, middleware)
	event := errors.New("fixed terminal error")
	route.Get("/event", func(ctx contractshttp.Context) contractshttp.Response {
		bound, err := middleware.EventContext(ctx.Context(), "terminal")
		if err != nil {
			t.Fatal(err)
		}
		writer(t, bound, driver).With(map[string]any{adapter.ExceptionKey: event, adapter.FramesKey: fixedFrames}).Error("original error log")
		other, err := middleware.EventContext(ctx.Context(), "other")
		if err != nil {
			t.Fatal(err)
		}
		writer(t, other, driver).With(map[string]any{adapter.ExceptionKey: event, adapter.FramesKey: fixedFrames}).Error("other real event")
		if err = middleware.MarkError(ctx.Context(), "terminal", event, fixedFrames); err != nil {
			t.Fatal(err)
		}
		return ctx.Response().String(503, "unchanged")
	})
	response := request(route, "GET", "/event", "")
	snapshots, err := store.Snapshot(context.Background())
	if response.Code != 503 || response.Body.String() != "unchanged" || err != nil || len(snapshots) != 1 || snapshots[0].Record.Count != 2 || snapshots[0].Record.Meta["source"] != "http_5xx" || snapshots[0].Record.Request.Method != "GET" || snapshots[0].Record.Request.URL == nil {
		t.Fatal("explicit IDs not respected", err)
	}
}
func TestHTTPRecoverDelegate(t *testing.T) {
	collector, store := setup(t)
	driver, _ := adapter.NewDriver(collector, &delegate{}, nil)
	callbacks := 0
	middleware, _ := adapter.NewHTTP(collector, adapter.HTTPConfig{Recover: func(ctx contractshttp.Context, value any) {
		callbacks++
		writer(t, ctx.Context(), driver).Error("owner callback log")
		if err := ctx.Response().String(502, "owner response").Abort(); err != nil {
			t.Fatal(err)
		}
	}})
	route := httpRoute(t, middleware)
	route.Get("/panic", func(contractshttp.Context) contractshttp.Response { panic(errors.New("token=fake panic")) })
	response := request(route, "GET", "/panic", "")
	snapshots, err := store.Snapshot(context.Background())
	if callbacks != 1 || response.Code != 502 || response.Body.String() != "owner response" || err != nil || len(snapshots) != 1 || snapshots[0].Record.Count != 1 || snapshots[0].Record.Meta["source"] != "http_5xx" || strings.Contains(snapshots[0].Record.Exception.Message, "fake") {
		t.Fatal("recover ownership/identity", err)
	}
}
func TestHTTPNoDelegateFrameworkRecover(t *testing.T) {
	collector, store := setup(t)
	middleware, _ := adapter.NewHTTP(collector, adapter.HTTPConfig{})
	route := httpRoute(t, middleware)
	native := logmocks.NewLog(t)
	native.On("WithContext", mock.Anything).Return(native)
	native.On("Request", mock.Anything).Return(native)
	native.On("Error", "fixed panic").Return()
	old := goravelgin.LogFacade
	goravelgin.LogFacade = native
	defer func() { goravelgin.LogFacade = old }()
	route.Get("/panic", func(contractshttp.Context) contractshttp.Response { panic("fixed panic") })
	response := request(route, "GET", "/panic", "")
	snapshots, err := store.Snapshot(context.Background())
	if response.Code != 500 || response.Body.Len() != 0 || err != nil || len(snapshots) != 0 {
		t.Fatal("framework recover replaced", err)
	}
}
func TestHTTPFailureIsolation(t *testing.T) {
	for _, mode := range []string{"collector", "identity"} {
		t.Run(mode, func(t *testing.T) {
			collector, store := setup(t)
			config := adapter.HTTPConfig{OnError: func(context.Context, adapter.Diagnostic) { panic("observer fake-token") }}
			if mode == "collector" {
				store.Close()
			} else {
				config.Identity = func(contractshttp.Context) (adapter.HTTPIdentity, error) {
					return adapter.HTTPIdentity{}, errors.New("fake-token")
				}
			}
			middleware, _ := adapter.NewHTTP(collector, config)
			route := httpRoute(t, middleware)
			route.Get("/error", func(ctx contractshttp.Context) contractshttp.Response { return ctx.Response().String(503, "business") })
			response := request(route, "GET", "/error", "")
			if response.Code != 503 || response.Body.String() != "business" || middleware.Status().Failures == 0 || !middleware.Status().Last.ObserverFailed {
				t.Fatal("monitoring failure changed business")
			}
		})
	}
}
func TestHTTPConcurrentRequests(t *testing.T) {
	collector, store := setup(t)
	middleware, _ := adapter.NewHTTP(collector, adapter.HTTPConfig{})
	route := httpRoute(t, middleware)
	route.Get("/error", func(ctx contractshttp.Context) contractshttp.Response { return ctx.Response().String(503, "business") })
	var group sync.WaitGroup
	for range 6 {
		group.Go(func() {
			response := request(route, "GET", "/error", "")
			if response.Code != 503 {
				t.Error("response changed")
			}
		})
	}
	group.Wait()
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 1 || snapshots[0].Record.Count != 6 || middleware.Status().Failures != 0 {
		t.Fatal("requests shared dedupe scope", err)
	}
}

func TestHTTPDelegatePanicAndCaptureFailure(t *testing.T) {
	for _, mode := range []string{"delegate-panic", "capture-failure"} {
		t.Run(mode, func(t *testing.T) {
			collector, store := setup(t)
			if mode == "capture-failure" {
				store.Close()
			}
			calls := 0
			middleware, _ := adapter.NewHTTP(collector, adapter.HTTPConfig{Recover: func(ctx contractshttp.Context, value any) {
				calls++
				if mode == "delegate-panic" {
					panic("owner panic")
				}
				if err := ctx.Response().String(502, "owner despite capture failure").Abort(); err != nil {
					t.Fatal(err)
				}
			}})
			route := httpRoute(t, middleware)
			var held context.Context
			route.Get("/panic", func(ctx contractshttp.Context) contractshttp.Response { held = ctx.Context(); panic("original") })
			native := logmocks.NewLog(t)
			old := goravelgin.LogFacade
			if mode == "delegate-panic" {
				native.On("WithContext", mock.Anything).Return(native)
				native.On("Request", mock.Anything).Return(native)
				native.On("Error", "owner panic").Return()
				goravelgin.LogFacade = native
			}
			defer func() { goravelgin.LogFacade = old }()
			response := request(route, "GET", "/panic", "")
			if calls != 1 {
				t.Fatal("owner callback skipped/repeated")
			}
			if mode == "delegate-panic" {
				if response.Code != 500 || response.Body.Len() != 0 {
					t.Fatal("owner panic swallowed")
				}
			} else if response.Code != 502 || response.Body.String() != "owner despite capture failure" || middleware.Status().Failures == 0 {
				t.Fatal("capture failure skipped owner")
			}
			if _, err := middleware.EventContext(held, "late"); !errors.Is(err, adapter.ErrHTTPContext) {
				t.Fatal("scope survived owner failure")
			}
		})
	}
}

func TestHTTPCommittedResponsePanic(t *testing.T) {
	collector, store := setup(t)
	middleware, _ := adapter.NewHTTP(collector, adapter.HTTPConfig{Recover: func(ctx contractshttp.Context, value any) {
		if err := ctx.Response().String(502, "owner suffix").Abort(); err != nil {
			t.Fatal(err)
		}
	}})
	route := httpRoute(t, middleware)
	route.Get("/panic", func(ctx contractshttp.Context) contractshttp.Response {
		if err := ctx.Response().String(202, "committed prefix;").Render(); err != nil {
			t.Fatal(err)
		}
		panic("after committed response")
	})
	response := request(route, "GET", "/panic", "")
	if response.Code != 202 || response.Body.String() != "committed prefix;owner suffix" {
		t.Fatal("owner committed-response behavior changed")
	}
	snapshots, err := store.Snapshot(context.Background())
	if err != nil || len(snapshots) != 1 || snapshots[0].Record.Count != 1 || snapshots[0].Record.Meta["source"] != "reportable" {
		t.Fatal("panic capture or actual final status observation", err)
	}
}
