package main

import (
	"io"
	"net/http"
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
