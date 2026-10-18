package main

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServerDropsSlowHeaders(t *testing.T) {
	srv := newServer("", nil)
	if srv.ReadHeaderTimeout == 0 || srv.IdleTimeout == 0 {
		t.Fatalf("expected header and idle timeouts, got %v and %v", srv.ReadHeaderTimeout, srv.IdleTimeout)
	}
	// Shorten the timeout so the test runs quickly
	srv.ReadHeaderTimeout = 100 * time.Millisecond

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send an incomplete request line and never finish the headers
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n")); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	start := time.Now()
	_, err = bufio.NewReader(conn).ReadString('\n')
	if err == nil {
		// The server may answer with a 408 before closing, which is also fine
		return
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("server kept the connection open for %v with incomplete headers", time.Since(start))
	}
}

func TestRunWaitsForInFlightRequests(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	srv := newServer("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-releaseRequest
		io.WriteString(w, "done")
	}))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- run(ctx, srv, ln) }()

	type result struct {
		body string
		err  error
	}
	response := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err != nil {
			response <- result{err: err}
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		response <- result{string(body), err}
	}()

	<-requestStarted
	cancel()

	// Shutdown must wait for the in-flight request rather than return straight away
	select {
	case err := <-runErr:
		t.Fatalf("run returned before the in-flight request finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseRequest)
	if r := <-response; r.err != nil || r.body != "done" {
		t.Errorf("expected in-flight request to complete with %q, got %q (err=%v)", "done", r.body, r.err)
	}
	if err := <-runErr; err != nil {
		t.Errorf("expected clean shutdown, got %v", err)
	}
	if _, err := net.Dial("tcp", ln.Addr().String()); err == nil {
		t.Error("expected listener to be closed after shutdown")
	}
}
