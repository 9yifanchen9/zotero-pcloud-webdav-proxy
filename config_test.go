package main

import (
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	validEnv := map[string]string{
		"PCLOUD_REGION": "us",
		"PCLOUD_USER":   "pclouduser",
		"PCLOUD_PASS":   "pcloudpass",
		"PCLOUD_ROOT":   "/Zotero",
		"PROXY_USER":    "testuser",
		"PROXY_PASS":    "testpass",
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
		if cfg.pcloudURL.String() != "https://webdav.pcloud.com" || cfg.pcloudUser != "pclouduser" ||
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

	for region, want := range map[string]string{"us": "https://webdav.pcloud.com", "eu": "https://ewebdav.pcloud.com", "EU": "https://ewebdav.pcloud.com"} {
		t.Run("selects the endpoint for region "+region, func(t *testing.T) {
			cfg, err := loadConfig(envWith(map[string]string{"PCLOUD_REGION": region}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.pcloudURL.String() != want {
				t.Errorf("expected %s, got %s", want, cfg.pcloudURL)
			}
		})
	}

	t.Run("fails for an unknown region", func(t *testing.T) {
		_, err := loadConfig(envWith(map[string]string{"PCLOUD_REGION": "asia"}))
		if err == nil || !strings.Contains(err.Error(), "PCLOUD_REGION") {
			t.Errorf("expected PCLOUD_REGION error, got %v", err)
		}
	})

	t.Run("fails when neither region nor URL is set", func(t *testing.T) {
		_, err := loadConfig(envWith(map[string]string{"PCLOUD_REGION": ""}))
		if err == nil || !strings.Contains(err.Error(), "PCLOUD_REGION") {
			t.Errorf("expected PCLOUD_REGION error, got %v", err)
		}
	})

	t.Run("fails when both region and URL are set", func(t *testing.T) {
		_, err := loadConfig(envWith(map[string]string{"PCLOUD_URL": "http://127.0.0.1:8099"}))
		if err == nil {
			t.Error("expected error when both PCLOUD_REGION and PCLOUD_URL are set")
		}
	})

	t.Run("uses PCLOUD_URL in place of a region", func(t *testing.T) {
		cfg, err := loadConfig(envWith(map[string]string{"PCLOUD_REGION": "", "PCLOUD_URL": "http://127.0.0.1:8099"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.pcloudURL.String() != "http://127.0.0.1:8099" {
			t.Errorf("expected PCLOUD_URL to be used, got %s", cfg.pcloudURL)
		}
	})

	for raw, want := range map[string]string{"/Zotero": "/Zotero", "Zotero": "/Zotero", "/Apps/Zotero/": "/Apps/Zotero", "/a/../Zotero": "/Zotero"} {
		t.Run("normalises PCLOUD_ROOT "+raw, func(t *testing.T) {
			cfg, err := loadConfig(envWith(map[string]string{"PCLOUD_ROOT": raw}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.pcloudRoot != want {
				t.Errorf("expected root %q, got %q", want, cfg.pcloudRoot)
			}
		})
	}

	for _, root := range []string{"/", "//", "/..", "."} {
		t.Run("fails for PCLOUD_ROOT at the drive root "+root, func(t *testing.T) {
			_, err := loadConfig(envWith(map[string]string{"PCLOUD_ROOT": root}))
			if err == nil || !strings.Contains(err.Error(), "PCLOUD_ROOT") {
				t.Errorf("expected PCLOUD_ROOT error, got %v", err)
			}
		})
	}

	for _, badURL := range []string{"webdav.pcloud.com", "ftp://webdav.pcloud.com", "https://", "://bad", "https://webdav.pcloud.com/Zotero", "https://webdav.pcloud.com/?a=b", "https://u:p@webdav.pcloud.com"} {
		t.Run("fails for invalid PCLOUD_URL "+badURL, func(t *testing.T) {
			_, err := loadConfig(envWith(map[string]string{"PCLOUD_REGION": "", "PCLOUD_URL": badURL}))
			if err == nil {
				t.Errorf("expected error for PCLOUD_URL %q", badURL)
			}
		})
	}

	t.Run("listens on loopback when LISTEN_ADDR is unset", func(t *testing.T) {
		cfg, err := loadConfig(envWith(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.listenAddr != "127.0.0.1:8080" {
			t.Errorf("expected 127.0.0.1:8080, got %q", cfg.listenAddr)
		}
	})

	for _, addr := range []string{"0.0.0.0:8080", ":8080", "[::1]:9000", "localhost:0"} {
		t.Run("accepts LISTEN_ADDR "+addr, func(t *testing.T) {
			cfg, err := loadConfig(envWith(map[string]string{"LISTEN_ADDR": addr}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.listenAddr != addr {
				t.Errorf("expected %q, got %q", addr, cfg.listenAddr)
			}
		})
	}

	for _, addr := range []string{"8080", "0.0.0.0", "0.0.0.0:", "0.0.0.0:http", "0.0.0.0:65536", "::1:8080"} {
		t.Run("fails for invalid LISTEN_ADDR "+addr, func(t *testing.T) {
			_, err := loadConfig(envWith(map[string]string{"LISTEN_ADDR": addr}))
			if err == nil || !strings.Contains(err.Error(), "LISTEN_ADDR") {
				t.Errorf("expected LISTEN_ADDR error, got %v", err)
			}
		})
	}
}
