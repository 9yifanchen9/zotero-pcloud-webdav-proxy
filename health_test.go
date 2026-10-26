package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestCheckHealth(t *testing.T) {
	// Serves the given handler on a loopback port and returns its address
	serve := func(t *testing.T, handler http.Handler) string {
		t.Helper()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		srv := newServer("", handler)
		go srv.Serve(ln)
		t.Cleanup(func() { srv.Close() })
		return ln.Addr().String()
	}

	t.Run("succeeds against a running proxy", func(t *testing.T) {
		addr := serve(t, newHandler("testuser", "testpass", http.NotFoundHandler()))
		if err := checkHealth(context.Background(), addr); err != nil {
			t.Errorf("expected healthy, got %v", err)
		}
	})

	t.Run("reaches a proxy listening on all interfaces over loopback", func(t *testing.T) {
		addr := serve(t, newHandler("testuser", "testpass", http.NotFoundHandler()))
		_, port, _ := net.SplitHostPort(addr)
		if err := checkHealth(context.Background(), "0.0.0.0:"+port); err != nil {
			t.Errorf("expected healthy, got %v", err)
		}
	})

	t.Run("fails when /healthz does not return 200", func(t *testing.T) {
		addr := serve(t, http.NotFoundHandler())
		if err := checkHealth(context.Background(), addr); err == nil || !strings.Contains(err.Error(), "404") {
			t.Errorf("expected a 404 error, got %v", err)
		}
	})

	t.Run("fails when nothing is listening", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := ln.Addr().String()
		ln.Close()
		if err := checkHealth(context.Background(), addr); err == nil {
			t.Error("expected an error with nothing listening")
		}
	})

	for listenAddr, want := range map[string]string{
		":8080":          "127.0.0.1:8080",
		"0.0.0.0:8080":   "127.0.0.1:8080",
		"[::]:8080":      "[::1]:8080",
		"127.0.0.1:8080": "127.0.0.1:8080",
		"10.0.0.5:8080":  "10.0.0.5:8080",
		"localhost:8080": "localhost:8080",
		"[fe80::1]:8080": "[fe80::1]:8080",
	} {
		t.Run("dials "+want+" for LISTEN_ADDR "+listenAddr, func(t *testing.T) {
			if got := healthCheckAddr(listenAddr); got != want {
				t.Errorf("expected %q, got %q", want, got)
			}
		})
	}
}
