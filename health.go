package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Path answered by the proxy itself so container runtimes can check it is up
const healthPath = "/healthz"

// Answers health checks before authentication, without contacting pCloud: an
// upstream outage should not get a working proxy restarted. Only the exact path
// matches, and it is not logged, as checks arrive every few seconds.
func withHealthCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != healthPath {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, "ok\n")
	})
}

// Requests /healthz from a running proxy, for images without curl or wget. Checks the
// address the proxy listens on, reaching an all-interfaces address over loopback.
func checkHealth(ctx context.Context, listenAddr string) error {
	client := &http.Client{
		Timeout: 3 * time.Second,
		// Never route the check through HTTP_PROXY
		Transport: &http.Transport{Proxy: nil},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+healthCheckAddr(listenAddr)+healthPath, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %s", resp.Status)
	}
	return nil
}

// Maps a listen address to one a client can dial: an empty or unspecified host
// becomes the loopback address of the same family
func healthCheckAddr(listenAddr string) string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return listenAddr
	}
	switch ip := net.ParseIP(host); {
	case host == "" || ip != nil && ip.Equal(net.IPv4zero):
		host = "127.0.0.1"
	case ip != nil && ip.Equal(net.IPv6unspecified):
		host = "::1"
	}
	return net.JoinHostPort(host, port)
}
