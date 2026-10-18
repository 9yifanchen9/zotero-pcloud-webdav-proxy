package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
	proxy := NewReverseProxy(pcloudUser, pcloudPass, serverURL)

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

func TestLoadConfig(t *testing.T) {
	validEnv := map[string]string{
		"PCLOUD_URL":  "https://webdav.pcloud.com",
		"PCLOUD_USER": "pclouduser",
		"PCLOUD_PASS": "pcloudpass",
		"PROXY_USER":  "testuser",
		"PROXY_PASS":  "testpass",
	}
	envWith := func(overrides map[string]string) func(string) string {
		return func(key string) string {
			if v, ok := overrides[key]; ok {
				return v
			}
			return validEnv[key]
		}
	}

	t.Run("loads all values from the environment", func(t *testing.T) {
		cfg, err := loadConfig(envWith(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.pcloudURL.String() != validEnv["PCLOUD_URL"] || cfg.pcloudUser != "pclouduser" ||
			cfg.pcloudPass != "pcloudpass" || cfg.proxyUser != "testuser" || cfg.proxyPass != "testpass" {
			t.Errorf("unexpected config: %+v", cfg)
		}
	})

	t.Run("fails listing every missing variable", func(t *testing.T) {
		_, err := loadConfig(envWith(map[string]string{"PCLOUD_PASS": "", "PROXY_PASS": ""}))
		if err == nil || !strings.Contains(err.Error(), "PCLOUD_PASS") || !strings.Contains(err.Error(), "PROXY_PASS") {
			t.Errorf("expected error naming PCLOUD_PASS and PROXY_PASS, got %v", err)
		}
	})

	for _, badURL := range []string{"webdav.pcloud.com", "ftp://webdav.pcloud.com", "https://", "://bad"} {
		t.Run("fails for invalid PCLOUD_URL "+badURL, func(t *testing.T) {
			_, err := loadConfig(envWith(map[string]string{"PCLOUD_URL": badURL}))
			if err == nil {
				t.Errorf("expected error for PCLOUD_URL %q", badURL)
			}
		})
	}
}
