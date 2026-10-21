package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// How long in-flight requests, such as large uploads, get to finish after a shutdown signal
const shutdownTimeout = 30 * time.Second

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

// Serves until ctx is cancelled, then stops accepting connections and waits for
// in-flight requests to finish
func run(ctx context.Context, srv *http.Server, ln net.Listener) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	log.Println("Shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func main() {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}

	proxy := NewReverseProxy(cfg.pcloudUser, cfg.pcloudPass, cfg.pcloudURL, cfg.pcloudRoot)
	handler := withRequestID(requireAuth(cfg.proxyUser, cfg.proxyPass, proxy))

	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("Begin listening")

	if err := run(ctx, newServer("", handler), ln); err != nil {
		log.Fatal(err)
	}
}
