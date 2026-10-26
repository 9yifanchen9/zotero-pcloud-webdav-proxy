package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Largest multistatus (207) body buffered for href rewriting. A PROPFIND of a Zotero
// folder lists two files per attachment, so this allows for very large libraries.
const maxMultistatusBytes = 32 << 20

// Matches <href> elements under any namespace prefix, e.g. <D:href>, capturing the
// opening tag, the text and the closing tag
var hrefPattern = regexp.MustCompile(`(?s)(<(?:[\w.-]+:)?href(?:\s[^>]*)?>)(.*?)(</(?:[\w.-]+:)?href\s*>)`)

// Proxies Zotero's WebDAV requests into a single folder on pCloud. The proxy's "/"
// maps to the configured root folder, so Zotero cannot reach anything outside it.
type webdavProxy struct {
	upstream     *url.URL // pCloud endpoint with the root folder as its path
	rootSegments []string
	proxy        *httputil.ReverseProxy
}

func handleUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
	logRequest(r, fmt.Sprintf("status=upstream-error err=%q", err))
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

// Builds the transport to pCloud from Go's default one, which brings dial and TLS
// handshake timeouts and honours HTTPS_PROXY, plus a bound on waiting for headers
func newUpstreamTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 30 * time.Second
	t.IdleConnTimeout = 90 * time.Second
	return t
}

func NewReverseProxy(user, pass string, target *url.URL, root string) http.Handler {
	upstream := *target
	upstream.Path = root
	upstream.RawPath = ""

	p := &webdavProxy{upstream: &upstream, rootSegments: strings.Split(root, "/")}
	p.proxy = &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(&upstream)
			r.Out.SetBasicAuth(user, pass)
			if dest := r.In.Header.Get("Destination"); dest != "" {
				destURL, _ := url.Parse(dest) // validated in ServeHTTP
				r.Out.Header.Set("Destination", p.toUpstream(destURL).String())
			}
			// Any request other than a download may get a multistatus body back, which
			// must be readable to rewrite. Without a client Accept-Encoding the transport
			// negotiates gzip itself and decodes the response transparently.
			if r.In.Method != http.MethodGet && r.In.Method != http.MethodHead {
				r.Out.Header.Del("Accept-Encoding")
			}
		},
		Transport:      newUpstreamTransport(),
		ErrorHandler:   handleUpstreamError,
		ModifyResponse: p.modifyResponse,
	}

	return p
}

func (p *webdavProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := validatePath(r.URL); err != nil {
		logRequest(r, fmt.Sprintf("status=rejected-path err=%q", err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// MOVE and COPY name their target in an absolute Destination URL on the proxy
	if dest := r.Header.Get("Destination"); dest != "" {
		destURL, err := url.Parse(dest)
		if err == nil {
			err = validatePath(destURL)
		}
		if err != nil {
			logRequest(r, fmt.Sprintf("status=rejected-destination err=%q", err))
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		if destURL.Host != "" && !strings.EqualFold(destURL.Host, r.Host) {
			// RFC 4918 section 9.8.5: a destination on another server is a 502
			logRequest(r, "status=rejected-destination err=\"destination is on another server\"")
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
			return
		}
	}

	p.proxy.ServeHTTP(w, r)
}

func (p *webdavProxy) modifyResponse(resp *http.Response) error {
	logRequest(resp.Request, fmt.Sprintf("status=%d", resp.StatusCode))

	if location := resp.Header.Get("Location"); location != "" {
		if mapped, ok := p.toProxy(location); ok {
			resp.Header.Set("Location", mapped)
		}
	}
	if resp.StatusCode == http.StatusMultiStatus {
		return p.rewriteHrefs(resp)
	}
	return nil
}

// Rewrites every href in a multistatus body from pCloud's paths to the proxy's. Zotero
// checks that the hrefs it gets back sit under the URL it asked for, so the root
// folder prefix must be removed.
func (p *webdavProxy) rewriteHrefs(resp *http.Response) error {
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return fmt.Errorf("cannot rewrite multistatus body with Content-Encoding %q", encoding)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMultistatusBytes+1))
	resp.Body.Close()
	if err != nil {
		return fmt.Errorf("reading multistatus body: %w", err)
	}
	if len(body) > maxMultistatusBytes {
		return fmt.Errorf("multistatus body exceeds %d bytes", maxMultistatusBytes)
	}

	body = hrefPattern.ReplaceAllFunc(body, func(match []byte) []byte {
		parts := hrefPattern.FindSubmatch(match)
		href := strings.TrimSpace(html.UnescapeString(string(parts[2])))
		mapped, ok := p.toProxy(href)
		if !ok {
			logRequest(resp.Request, fmt.Sprintf("status=unmapped-href href=%q", href))
			return match
		}
		var out bytes.Buffer
		out.Write(parts[1])
		xml.EscapeText(&out, []byte(mapped))
		out.Write(parts[3])
		return out.Bytes()
	})

	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.TransferEncoding = nil
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}

// Maps a proxy URL, such as a Destination header, to the same resource on pCloud
func (p *webdavProxy) toUpstream(u *url.URL) *url.URL {
	out := *p.upstream
	out.Path = p.upstream.Path + u.Path
	out.RawPath = p.upstream.EscapedPath() + u.EscapedPath()
	return &out
}

// Maps a URL or path returned by pCloud back to a path on the proxy. Reports false
// for anything outside the root folder or on another host.
func (p *webdavProxy) toProxy(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host != "" && !strings.EqualFold(u.Host, p.upstream.Host) {
		return "", false
	}
	// Compare decoded segments, as pCloud may escape the root differently from Go
	segments := strings.Split(u.EscapedPath(), "/")
	if len(segments) < len(p.rootSegments) {
		return "", false
	}
	for i, want := range p.rootSegments {
		got, err := url.PathUnescape(segments[i])
		if err != nil || got != want {
			return "", false
		}
	}
	return "/" + strings.Join(segments[len(p.rootSegments):], "/"), true
}

// Rejects paths that could resolve outside the root folder once joined onto it.
// httputil.ReverseProxy forwards paths as-is, and http.ServeMux's path cleaning only
// applies when routing through a mux (and answers with a redirect), so the check is
// done explicitly here. Both the decoded path and its escaped form are checked, as
// pCloud may decode escapes such as %2F that Go keeps as part of a segment.
func validatePath(u *url.URL) error {
	if !strings.HasPrefix(u.Path, "/") {
		return errors.New("path must be absolute")
	}
	if strings.ContainsAny(u.Path, "\\\x00") {
		return errors.New("path contains a backslash or NUL byte")
	}
	for segment := range strings.SplitSeq(u.Path, "/") {
		if segment == "." || segment == ".." {
			return errors.New("path contains a dot segment")
		}
	}
	escaped := strings.ToLower(u.EscapedPath())
	if strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%5c") {
		return errors.New("path contains an encoded separator")
	}
	return nil
}
