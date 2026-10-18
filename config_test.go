package main

import (
	"strings"
	"testing"
)

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
