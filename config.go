package main

import (
	"fmt"
	"net/url"
)

type config struct {
	pcloudURL  *url.URL
	pcloudUser string
	pcloudPass string
	proxyUser  string
	proxyPass  string
}

// Reads proxy configuration from the environment, failing if any value is missing
func loadConfig(getenv func(string) string) (*config, error) {
	values := map[string]string{}
	var missing []string
	for _, key := range []string{"PCLOUD_URL", "PCLOUD_USER", "PCLOUD_PASS", "PROXY_USER", "PROXY_PASS"} {
		values[key] = getenv(key)
		if values[key] == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missing)
	}

	target, err := url.Parse(values["PCLOUD_URL"])
	if err != nil {
		return nil, fmt.Errorf("invalid PCLOUD_URL: %w", err)
	}
	if target.Scheme != "http" && target.Scheme != "https" || target.Host == "" {
		return nil, fmt.Errorf("invalid PCLOUD_URL %q: must be an absolute http(s) URL", values["PCLOUD_URL"])
	}

	return &config{
		pcloudURL:  target,
		pcloudUser: values["PCLOUD_USER"],
		pcloudPass: values["PCLOUD_PASS"],
		proxyUser:  values["PROXY_USER"],
		proxyPass:  values["PROXY_PASS"],
	}, nil
}
