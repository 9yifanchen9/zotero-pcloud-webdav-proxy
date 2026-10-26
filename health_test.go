package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	var upstreamHits int
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
	}))
	server.Start()
	serverURL, _ := url.Parse(server.URL)
	handler := newHandler("testuser", "testpass", NewReverseProxy("pclouduser", "pcloudpass", serverURL, "/Zotero"))

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run("answers "+method+" /healthz without credentials or contacting pCloud", func(t *testing.T) {
			upstreamHits = 0
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, httptest.NewRequest(method, "/healthz", nil))

			if response.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", response.Code)
			}
			if upstreamHits != 0 {
				t.Error("health check reached the upstream")
			}
		})
	}

	t.Run("returns ok in the body", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if body, _ := io.ReadAll(response.Body); string(body) != "ok\n" {
			t.Errorf("expected body %q, got %q", "ok\n", body)
		}
	})

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, "PROPFIND"} {
		t.Run("rejects "+method+" /healthz with 405", func(t *testing.T) {
			upstreamHits = 0
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, httptest.NewRequest(method, "/healthz", nil))

			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, HEAD" {
				t.Errorf("expected 405 with Allow: GET, HEAD, got %d with %q", response.Code, response.Header().Get("Allow"))
			}
			if upstreamHits != 0 {
				t.Error("health check reached the upstream")
			}
		})
	}

	for _, requestPath := range []string{"/healthz/", "/HEALTHZ", "/zotero/healthz", "/healthz.prop"} {
		t.Run("requires credentials for "+requestPath, func(t *testing.T) {
			upstreamHits = 0
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))

			if response.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", response.Code)
			}
		})

		t.Run("proxies "+requestPath+" to pCloud once authenticated", func(t *testing.T) {
			upstreamHits = 0
			request := httptest.NewRequest(http.MethodGet, requestPath, nil)
			request.SetBasicAuth("testuser", "testpass")
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK || upstreamHits != 1 {
				t.Errorf("expected 200 from the upstream, got %d with %d upstream hits", response.Code, upstreamHits)
			}
		})
	}
}
