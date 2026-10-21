package main

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

// pCloud WebDAV endpoints by account region. An account only works against the
// endpoint for the region it was created in.
var pcloudRegionURLs = map[string]string{
	"us": "https://webdav.pcloud.com",
	"eu": "https://ewebdav.pcloud.com",
}

type config struct {
	pcloudURL  *url.URL
	pcloudRoot string
	pcloudUser string
	pcloudPass string
	proxyUser  string
	proxyPass  string
}

// Reads proxy configuration from the environment, failing if any value is missing
func loadConfig(getenv func(string) string) (*config, error) {
	values := map[string]string{}
	var missing []string
	for _, key := range []string{"PCLOUD_USER", "PCLOUD_PASS", "PCLOUD_ROOT", "PROXY_USER", "PROXY_PASS"} {
		values[key] = getenv(key)
		if values[key] == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missing)
	}

	target, err := upstreamURL(getenv("PCLOUD_REGION"), getenv("PCLOUD_URL"))
	if err != nil {
		return nil, err
	}

	// Zotero is confined to this folder, so the drive root is not allowed
	root := path.Clean("/" + values["PCLOUD_ROOT"])
	if root == "/" {
		return nil, errors.New("invalid PCLOUD_ROOT: must be a folder, not the root of the drive")
	}

	return &config{
		pcloudURL:  target,
		pcloudRoot: root,
		pcloudUser: values["PCLOUD_USER"],
		pcloudPass: values["PCLOUD_PASS"],
		proxyUser:  values["PROXY_USER"],
		proxyPass:  values["PROXY_PASS"],
	}, nil
}

// Picks the upstream from PCLOUD_REGION, or from PCLOUD_URL when pointing at another
// WebDAV server such as a local test server. Exactly one of the two must be set.
func upstreamURL(region, rawURL string) (*url.URL, error) {
	switch {
	case region != "" && rawURL != "":
		return nil, errors.New("set only one of PCLOUD_REGION and PCLOUD_URL")
	case region != "":
		regionURL, ok := pcloudRegionURLs[strings.ToLower(region)]
		if !ok {
			return nil, fmt.Errorf("invalid PCLOUD_REGION %q: must be us or eu", region)
		}
		return url.Parse(regionURL)
	case rawURL != "":
		target, err := url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("invalid PCLOUD_URL: %w", err)
		}
		if target.Scheme != "http" && target.Scheme != "https" || target.Host == "" {
			return nil, fmt.Errorf("invalid PCLOUD_URL %q: must be an absolute http(s) URL", rawURL)
		}
		if target.Path != "" && target.Path != "/" || target.RawQuery != "" || target.User != nil {
			return nil, fmt.Errorf("invalid PCLOUD_URL %q: must not include a path, query or credentials; set the folder with PCLOUD_ROOT", rawURL)
		}
		target.Path = ""
		return target, nil
	default:
		return nil, errors.New("missing required environment variable: set PCLOUD_REGION (us or eu)")
	}
}
