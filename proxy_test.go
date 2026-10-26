package main

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// Starts a fake pCloud and returns a proxy rooted at /Zotero in front of it
func newTestProxy(t *testing.T, upstream http.HandlerFunc) http.Handler {
	t.Helper()
	server := httptest.NewTestServer(t, upstream)
	server.Start()
	serverURL, _ := url.Parse(server.URL)
	return NewReverseProxy("pclouduser", "pcloudpass", serverURL, "/Zotero")
}

func TestPathScoping(t *testing.T) {
	var upstreamPath string
	var upstreamHit bool
	proxy := newTestProxy(t, func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		upstreamPath = r.URL.EscapedPath()
	})

	allowed := map[string]string{
		"/":                     "/Zotero/",
		"/zotero/":              "/Zotero/zotero/",
		"/zotero/ABCD1234.zip":  "/Zotero/zotero/ABCD1234.zip",
		"/zotero/a%20b.prop":    "/Zotero/zotero/a%20b.prop",
		"/zotero/..hidden.prop": "/Zotero/zotero/..hidden.prop",
	}
	for requestPath, want := range allowed {
		t.Run("maps "+requestPath+" into the root folder", func(t *testing.T) {
			upstreamHit, upstreamPath = false, ""
			response := httptest.NewRecorder()

			proxy.ServeHTTP(response, httptest.NewRequest("PROPFIND", requestPath, nil))

			if response.Code != http.StatusOK || upstreamPath != want {
				t.Errorf("expected 200 with upstream path %q, got %d with %q", want, response.Code, upstreamPath)
			}
		})
	}

	rejected := []string{
		"/zotero/../secret",
		"/..",
		"/zotero/./x",
		"/zotero/%2e%2e/secret",
		"/zotero/%2E%2E/secret",
		"/zotero/..%2Fsecret",
		"/zotero/%2F..%2Fsecret",
		"/zotero/..%5Csecret",
		"/zotero/a%00b",
	}
	for _, requestPath := range rejected {
		t.Run("rejects "+requestPath, func(t *testing.T) {
			upstreamHit = false
			response := httptest.NewRecorder()

			proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))

			if response.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", response.Code)
			}
			if upstreamHit {
				t.Error("rejected request reached the upstream")
			}
		})
	}
}

func TestDestinationRewrite(t *testing.T) {
	var upstreamDestination string
	var upstreamHit bool
	var upstreamHost string
	proxy := newTestProxy(t, func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		upstreamHost = r.Host
		upstreamDestination = r.Header.Get("Destination")
		w.WriteHeader(http.StatusCreated)
	})

	move := func(destination string) *httptest.ResponseRecorder {
		upstreamHit, upstreamDestination = false, ""
		request := httptest.NewRequest("MOVE", "http://proxy.local/zotero/old.zip", nil)
		request.Header.Set("Destination", destination)
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, request)
		return response
	}

	for _, destination := range []string{"http://proxy.local/zotero/new%20name.zip", "/zotero/new%20name.zip"} {
		t.Run("rewrites destination "+destination+" into the root folder", func(t *testing.T) {
			response := move(destination)

			want := "http://" + upstreamHost + "/Zotero/zotero/new%20name.zip"
			if response.Code != http.StatusCreated || upstreamDestination != want {
				t.Errorf("expected 201 with Destination %q, got %d with %q", want, response.Code, upstreamDestination)
			}
		})
	}

	t.Run("rejects a destination that escapes the root folder", func(t *testing.T) {
		response := move("http://proxy.local/zotero/../../secret")

		if response.Code != http.StatusBadRequest || upstreamHit {
			t.Errorf("expected 400 without reaching upstream, got %d (upstream hit=%v)", response.Code, upstreamHit)
		}
	})

	t.Run("rejects a destination on another server", func(t *testing.T) {
		response := move("http://elsewhere.example/zotero/new.zip")

		if response.Code != http.StatusBadGateway || upstreamHit {
			t.Errorf("expected 502 without reaching upstream, got %d (upstream hit=%v)", response.Code, upstreamHit)
		}
	})
}

func TestLocationRewrite(t *testing.T) {
	proxy := newTestProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://"+r.Host+"/Zotero/zotero/ABCD1234.zip")
		w.WriteHeader(http.StatusCreated)
	})
	response := httptest.NewRecorder()

	proxy.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/zotero/ABCD1234.zip", strings.NewReader("data")))

	if got := response.Header().Get("Location"); got != "/zotero/ABCD1234.zip" {
		t.Errorf("expected Location /zotero/ABCD1234.zip, got %q", got)
	}
}

func TestMultistatusRewrite(t *testing.T) {
	const multistatus = `<?xml version="1.0" encoding="utf-8"?>
<D:multistatus xmlns:D="DAV:">
  <D:response><D:href>http://HOST/Zotero/zotero/</D:href></D:response>
  <D:response><D:href>/Zotero/zotero/ABCD1234.zip</D:href></D:response>
  <D:response><D:href> /Zotero/zotero/a%20b&amp;c.prop </D:href></D:response>
  <response xmlns="DAV:"><href>/%5Aotero/zotero/escaped-root.prop</href></response>
  <D:response><D:href>/Other/secret.txt</D:href></D:response>
</D:multistatus>`
	const want = `<?xml version="1.0" encoding="utf-8"?>
<D:multistatus xmlns:D="DAV:">
  <D:response><D:href>/zotero/</D:href></D:response>
  <D:response><D:href>/zotero/ABCD1234.zip</D:href></D:response>
  <D:response><D:href>/zotero/a%20b&amp;c.prop</D:href></D:response>
  <response xmlns="DAV:"><href>/zotero/escaped-root.prop</href></response>
  <D:response><D:href>/Other/secret.txt</D:href></D:response>
</D:multistatus>`

	for _, compress := range []bool{false, true} {
		name := "rewrites hrefs into proxy paths"
		if compress {
			name += " from a gzip response"
		}
		t.Run(name, func(t *testing.T) {
			proxy := newTestProxy(t, func(w http.ResponseWriter, r *http.Request) {
				body := strings.ReplaceAll(multistatus, "HOST", r.Host)
				w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
				if compress && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
					w.Header().Set("Content-Encoding", "gzip")
					w.WriteHeader(http.StatusMultiStatus)
					gz := gzip.NewWriter(w)
					io.WriteString(gz, body)
					gz.Close()
					return
				}
				w.WriteHeader(http.StatusMultiStatus)
				io.WriteString(w, body)
			})
			request := httptest.NewRequest("PROPFIND", "/zotero/", nil)
			request.Header.Set("Depth", "1")
			// Zotero's HTTP stack asks for compressed responses
			request.Header.Set("Accept-Encoding", "gzip, deflate, br")
			response := httptest.NewRecorder()

			proxy.ServeHTTP(response, request)

			got := response.Body.String()
			if response.Code != http.StatusMultiStatus || got != want {
				t.Errorf("expected 207 with\n%s\ngot %d with\n%s", want, response.Code, got)
			}
			if response.Header().Get("Content-Encoding") != "" {
				t.Errorf("expected decoded body, got Content-Encoding %q", response.Header().Get("Content-Encoding"))
			}
			if cl := response.Header().Get("Content-Length"); cl != strconv.Itoa(len(want)) {
				t.Errorf("expected Content-Length %d, got %q", len(want), cl)
			}
		})
	}

	t.Run("fails with 502 rather than pass on an unrewritable body", func(t *testing.T) {
		proxy := newTestProxy(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Encoding", "br")
			w.WriteHeader(http.StatusMultiStatus)
			io.WriteString(w, "not really brotli")
		})
		response := httptest.NewRecorder()

		proxy.ServeHTTP(response, httptest.NewRequest("PROPFIND", "/zotero/", nil))

		if response.Code != http.StatusBadGateway {
			t.Errorf("expected 502, got %d", response.Code)
		}
	})
}

func TestUpstreamUnreachable(t *testing.T) {
	// Reserve a port, then close it so nothing is listening there
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadURL, _ := url.Parse("http://" + ln.Addr().String())
	ln.Close()

	captureLogs(t)
	proxy := NewReverseProxy("pclouduser", "pcloudpass", deadURL, "/Zotero")
	response := httptest.NewRecorder()

	proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/zotero/ABCD1234.zip", nil))

	if response.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", response.Code)
	}
}

func TestBodiesPassThrough(t *testing.T) {
	type received struct {
		method string
		body   []byte
	}
	var got received
	download := make([]byte, 4<<20)
	rand.Read(download)
	proxy := newTestProxy(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = received{r.Method, body}
		if r.Method == http.MethodGet {
			w.Write(download)
		}
	})

	upload := make([]byte, 4<<20)
	rand.Read(upload)
	propfind := []byte(`<?xml version="1.0"?><propfind xmlns="DAV:"><prop><getlastmodified/></prop></propfind>`)

	for _, tc := range []struct {
		method string
		body   []byte
	}{
		{http.MethodPut, upload},
		{"PROPFIND", propfind},
		{"MKCOL", nil},
		{http.MethodDelete, nil},
		{http.MethodOptions, nil},
	} {
		t.Run(tc.method+" request body reaches upstream intact", func(t *testing.T) {
			got = received{}
			response := httptest.NewRecorder()

			proxy.ServeHTTP(response, httptest.NewRequest(tc.method, "/zotero/ABCD1234.zip", bytes.NewReader(tc.body)))

			if response.Code != http.StatusOK || got.method != tc.method || !bytes.Equal(got.body, tc.body) {
				t.Errorf("expected 200 with %s and a %d byte body upstream, got %d with %s and %d bytes",
					tc.method, len(tc.body), response.Code, got.method, len(got.body))
			}
		})
	}

	t.Run("GET response body reaches Zotero intact", func(t *testing.T) {
		response := httptest.NewRecorder()

		proxy.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/zotero/ABCD1234.zip", nil))

		if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), download) {
			t.Errorf("expected 200 with the %d byte download, got %d with %d bytes", len(download), response.Code, response.Body.Len())
		}
	})
}

func TestUpstreamTransport(t *testing.T) {
	transport := newUpstreamTransport()

	t.Run("honours proxy environment variables", func(t *testing.T) {
		if transport.Proxy == nil {
			t.Error("expected a Proxy function such as http.ProxyFromEnvironment")
		}
	})

	t.Run("bounds dialling, the TLS handshake and waiting for headers", func(t *testing.T) {
		if transport.DialContext == nil || transport.TLSHandshakeTimeout == 0 || transport.ResponseHeaderTimeout == 0 {
			t.Errorf("expected dial, TLS handshake and response header timeouts, got DialContext=%v TLSHandshakeTimeout=%v ResponseHeaderTimeout=%v",
				transport.DialContext != nil, transport.TLSHandshakeTimeout, transport.ResponseHeaderTimeout)
		}
	})
}
