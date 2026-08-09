package httpservice

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gatehouse/config"
)

func TestHandlerEnablesConfiguredRouteGroups(t *testing.T) {
	handler := Handler(config.HTTPService{Web: true, API: true})
	for _, test := range []struct {
		path string
		body string
	}{
		{path: "/healthz", body: "ok\n"},
		{path: "/readyz", body: "ok\n"},
		{path: "/", body: "<!doctype html><title>Gatehouse</title><h1>Gatehouse</h1>\n"},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", test.path, response.Code, http.StatusOK)
			}
			if response.Body.String() != test.body {
				t.Fatalf("GET %s body = %q, want %q", test.path, response.Body.String(), test.body)
			}
		})
	}
}

func TestHandlerServesHealthWithoutRouteGroups(t *testing.T) {
	handler := Handler(config.HTTPService{})
	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK || response.Body.String() != "ok\n" {
				t.Fatalf("GET %s = status %d body %q, want status %d body %q", path, response.Code, response.Body.String(), http.StatusOK, "ok\n")
			}
		})
	}
}

func TestHandlerDisablesWebRouteGroup(t *testing.T) {
	response := httptest.NewRecorder()
	Handler(config.HTTPService{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("GET / status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestServiceStartsServesAndStops(t *testing.T) {
	err, service := Start(config.HTTPService{Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}

	response, err := http.Get("http://" + service.Address() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("GET health = status %d body %q", response.StatusCode, body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := service.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-service.Done(); err != nil {
		t.Fatal(err)
	}
}
