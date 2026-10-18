package main

import (
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
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

// Handler on authentication from Zotero WebDAV
func requireAuth(user, pass string, next http.Handler) http.Handler {
	authHandlerFunc := func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		userIsValid := subtle.ConstantTimeCompare([]byte(u), []byte(user)) == 1
		passIsValid := subtle.ConstantTimeCompare([]byte(p), []byte(pass)) == 1

		if !ok || !userIsValid || !passIsValid {
			w.Header().Set("WWW-Authenticate", `Basic realm="zotero-webdav"`)
			log.Printf("method=%s path=%s status=unauthorised-from-zotero", r.Method, r.URL.Path)
			http.Error(w, "Unauthorised", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(authHandlerFunc)
}

func handleUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("upstream error: method=%s path=%s err=%v", r.Method, r.URL.Path, err)
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

func logResponse(resp *http.Response) error {
	log.Printf("method=%s path=%s status=%d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode)
	return nil
}

func NewReverseProxy(user, pass string, target *url.URL) *httputil.ReverseProxy {
	customTransport := &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.SetBasicAuth(user, pass)
		},
		Transport:      customTransport,
		ErrorHandler:   handleUpstreamError,
		ModifyResponse: logResponse,
	}

	return proxy
}

func main() {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}

	proxy := NewReverseProxy(cfg.pcloudUser, cfg.pcloudPass, cfg.pcloudURL)
	handler := requireAuth(cfg.proxyUser, cfg.proxyPass, proxy)
	http.Handle("/", handler)

	log.Println("Begin listening")

	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}

}
