package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Proxies Zotero's WebDAV requests into a single folder on pCloud. The proxy's "/"
// maps to the configured root folder, so Zotero cannot reach anything outside it.
type webdavProxy struct {
	upstream *url.URL // pCloud endpoint with the root folder as its path
	proxy    *httputil.ReverseProxy
}

func handleUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
	logRequest(r, fmt.Sprintf("status=upstream-error err=%q", err))
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

func logResponse(resp *http.Response) error {
	logRequest(resp.Request, fmt.Sprintf("status=%d", resp.StatusCode))
	return nil
}

func NewReverseProxy(user, pass string, target *url.URL, root string) http.Handler {
	upstream := *target
	upstream.Path = root
	upstream.RawPath = ""

	customTransport := &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(&upstream)
			r.Out.SetBasicAuth(user, pass)
		},
		Transport:      customTransport,
		ErrorHandler:   handleUpstreamError,
		ModifyResponse: logResponse,
	}

	return &webdavProxy{upstream: &upstream, proxy: proxy}
}

func (p *webdavProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := validatePath(r.URL); err != nil {
		logRequest(r, fmt.Sprintf("status=rejected-path err=%q", err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	p.proxy.ServeHTTP(w, r)
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
