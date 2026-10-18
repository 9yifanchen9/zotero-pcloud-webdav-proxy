package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

// Builds the listening server. There is deliberately no ReadTimeout or WriteTimeout:
// they bound the whole request/response including the body, which would cut off large
// attachment uploads and downloads on slow links. Bodies are only read after
// authentication, so an unauthenticated client cannot hold a connection open by
// trickling one.
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
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

	err = newServer(":8080", nil).ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}

}
