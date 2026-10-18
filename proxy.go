package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

func handleUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("upstream error: method=%s path=%s err=%v", r.Method, r.URL.Path, err)
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

func logResponse(resp *http.Response) error {
	log.Printf("method=%s path=%s status=%d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode)
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
