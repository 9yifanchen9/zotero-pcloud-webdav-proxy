package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

func handleUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
	logRequest(r, fmt.Sprintf("status=upstream-error err=%q", err))
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

func logResponse(resp *http.Response) error {
	logRequest(resp.Request, fmt.Sprintf("status=%d", resp.StatusCode))
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
