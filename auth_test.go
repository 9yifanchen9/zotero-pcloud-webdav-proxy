package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestAuthHandler(t *testing.T) {
	pcloudUser, pcloudPass := "pclouduser", "pcloudpass"
	var upstreamUser, upstreamPass string
	var upstreamAuthOK bool
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamUser, upstreamPass, upstreamAuthOK = r.BasicAuth()
	}))
	server.Start()
	serverURL, _ := url.Parse(server.URL)
	proxy := NewReverseProxy(pcloudUser, pcloudPass, serverURL, "/Zotero")

	user, pass := "testuser", "testpass"
	handler := requireAuth(user, pass, proxy)

	assertUnauthorised := func(t *testing.T, results *http.Response) {
		t.Helper()
		if results.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected unauthorised, got %v", results.Status)
		}
		if got, want := results.Header.Get("WWW-Authenticate"), `Basic realm="zotero-webdav"`; got != want {
			t.Errorf("expected WWW-Authenticate %q, got %q", want, got)
		}
	}

	t.Run("fail authentication for missing authorisation header", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/zotero", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assertUnauthorised(t, response.Result())
	})

	t.Run("fail authentication for incorrect credentials", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/zotero", nil)
		request.SetBasicAuth("incorrectuser", "incorrectpassword")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assertUnauthorised(t, response.Result())
	})

	t.Run("fail authentication for correct user with incorrect password", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/zotero", nil)
		request.SetBasicAuth(user, "incorrectpassword")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assertUnauthorised(t, response.Result())
	})

	for _, nearMiss := range []string{"testpas", "testpass1", ""} {
		t.Run("fail authentication for near-miss password "+nearMiss, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/zotero", nil)
			request.SetBasicAuth(user, nearMiss)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			assertUnauthorised(t, response.Result())
		})
	}

	t.Run("successful proxy with pcloud credentials for correct credentials", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/zotero", nil)
		request.SetBasicAuth(user, pass)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		results := response.Result()
		if results.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %v", results.Status)
		}
		if !upstreamAuthOK || upstreamUser != pcloudUser || upstreamPass != pcloudPass {
			t.Errorf("expected upstream credentials %q/%q, got %q/%q (ok=%v)",
				pcloudUser, pcloudPass, upstreamUser, upstreamPass, upstreamAuthOK)
		}
	})
}
