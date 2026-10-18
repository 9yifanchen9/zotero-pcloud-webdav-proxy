package main

import (
	"bufio"
	"net"
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
