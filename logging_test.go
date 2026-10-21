package main

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func TestRequestLogging(t *testing.T) {
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Start()
	serverURL, _ := url.Parse(server.URL)

	pcloudUser, pcloudPass := "pclouduser", "pcloud-secret"
	user, pass := "testuser", "proxy-secret"
	handler := withRequestID(requireAuth(user, pass, NewReverseProxy(pcloudUser, pcloudPass, serverURL, "/Zotero")))

	t.Run("logs the request ID for proxied requests without credentials", func(t *testing.T) {
		logs := captureLogs(t)
		request := httptest.NewRequest(http.MethodGet, "/zotero/ABC.zip", nil)
		request.SetBasicAuth(user, pass)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		id := response.Result().Header.Get("X-Request-ID")
		if len(id) != 16 {
			t.Fatalf("expected a 16 character X-Request-ID, got %q", id)
		}
		if !strings.Contains(logs.String(), "id="+id+" ") || !strings.Contains(logs.String(), "status=200") {
			t.Errorf("expected log line with id=%s and status=200, got %q", id, logs.String())
		}
		for _, secret := range []string{pass, pcloudPass} {
			if strings.Contains(logs.String(), secret) {
				t.Errorf("log output contains a credential: %q", logs.String())
			}
		}
	})

	t.Run("logs the request ID for rejected requests without credentials", func(t *testing.T) {
		logs := captureLogs(t)
		request := httptest.NewRequest(http.MethodGet, "/zotero/ABC.zip", nil)
		request.SetBasicAuth(user, "wrong-secret")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		id := response.Result().Header.Get("X-Request-ID")
		if !strings.Contains(logs.String(), "id="+id+" ") || !strings.Contains(logs.String(), "unauthorised") {
			t.Errorf("expected unauthorised log line with id=%s, got %q", id, logs.String())
		}
		if strings.Contains(logs.String(), "wrong-secret") {
			t.Errorf("log output contains the attempted password: %q", logs.String())
		}
	})

	t.Run("gives each request a different ID", func(t *testing.T) {
		captureLogs(t)
		ids := map[string]bool{}
		for range 10 {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
			ids[response.Result().Header.Get("X-Request-ID")] = true
		}
		if len(ids) != 10 {
			t.Errorf("expected 10 distinct request IDs, got %d", len(ids))
		}
	})

	t.Run("quotes paths so they cannot forge log lines", func(t *testing.T) {
		logs := captureLogs(t)
		request := httptest.NewRequest(http.MethodGet, "/zotero/a%0Aid=forged", nil)
		handler.ServeHTTP(httptest.NewRecorder(), request)

		if lines := strings.Count(strings.TrimRight(logs.String(), "\n"), "\n"); lines != 0 {
			t.Errorf("expected a single log line, got %q", logs.String())
		}
	})
}
