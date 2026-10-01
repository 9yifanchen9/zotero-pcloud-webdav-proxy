package main

import (
	"testing"
	"net/url"
	"net/http"
	"net/http/httputil"
	"net/http/httptest"
)

func TestAuthHandler(t *testing.T) {
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Start()
	serverURL, _ := url.Parse(server.URL)
	proxy := httputil.NewSingleHostReverseProxy(serverURL)

	user, pass := "testuser", "testpass"
	handler := requireAuth(user, pass, proxy)

	t.Run("fail authentication for missing authorisation header", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/zotero", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		results := response.Result()

		if results.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected unauthorised, got %v", results.Status)
		}
	})

	t.Run("fail authentication for incorrect credentials", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/zotero", nil)
		request.SetBasicAuth("incorrectuser", "incorrectpassword")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		results := response.Result()

		if results.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected unauthorised, got %v", results.Status)
		}
	})

	t.Run("successful proxy for correct credentials", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/zotero", nil)
		request.SetBasicAuth(user, pass)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		results := response.Result()
		if results.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %v", results.Status)
		}
	})
}
