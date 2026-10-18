package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// Starts a fake pCloud and returns a proxy rooted at /Zotero in front of it
func newTestProxy(t *testing.T, upstream http.HandlerFunc) http.Handler {
	t.Helper()
	server := httptest.NewTestServer(t, upstream)
	server.Start()
	serverURL, _ := url.Parse(server.URL)
	return NewReverseProxy("pclouduser", "pcloudpass", serverURL, "/Zotero")
}

func TestPathScoping(t *testing.T) {
	var upstreamPath string
	var upstreamHit bool
	proxy := newTestProxy(t, func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		upstreamPath = r.URL.EscapedPath()
	})

	allowed := map[string]string{
		"/":                     "/Zotero/",
		"/zotero/":              "/Zotero/zotero/",
		"/zotero/ABCD1234.zip":  "/Zotero/zotero/ABCD1234.zip",
		"/zotero/a%20b.prop":    "/Zotero/zotero/a%20b.prop",
		"/zotero/..hidden.prop": "/Zotero/zotero/..hidden.prop",
	}
	for requestPath, want := range allowed {
		t.Run("maps "+requestPath+" into the root folder", func(t *testing.T) {
			upstreamHit, upstreamPath = false, ""
			response := httptest.NewRecorder()

			proxy.ServeHTTP(response, httptest.NewRequest("PROPFIND", requestPath, nil))

			if response.Code != http.StatusOK || upstreamPath != want {
				t.Errorf("expected 200 with upstream path %q, got %d with %q", want, response.Code, upstreamPath)
			}
		})
	}

	rejected := []string{
		"/zotero/../secret",
		"/..",
		"/zotero/./x",
		"/zotero/%2e%2e/secret",
		"/zotero/%2E%2E/secret",
		"/zotero/..%2Fsecret",
		"/zotero/%2F..%2Fsecret",
		"/zotero/..%5Csecret",
		"/zotero/a%00b",
	}
	for _, requestPath := range rejected {
		t.Run("rejects "+requestPath, func(t *testing.T) {
			upstreamHit = false
			response := httptest.NewRecorder()

			proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))

			if response.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", response.Code)
			}
			if upstreamHit {
				t.Error("rejected request reached the upstream")
			}
		})
	}
}
