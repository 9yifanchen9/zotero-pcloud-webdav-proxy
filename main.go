package main

import (
	"log"
	"time"
	"crypto/subtle"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// Handler on authentication from Zotero WebDAV
func requireAuth(user, pass string, next http.Handler) http.Handler {
	authHandlerFunc := func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		userIsValid := subtle.ConstantTimeCompare([]byte(u), []byte(user)) == 1
		passIsValid := subtle.ConstantTimeCompare([]byte(p), []byte(pass)) == 1

		if !ok || !userIsValid || !passIsValid {
			w.Header().Set("WWW-Authenticate", `Basic realm="zotero-webdav`)
			log.Printf("method=%s path=%s status=unauthorised-from-zotero", r.Method, r.URL.Path)
			http.Error(w, "Unauthorised", http.StatusUnauthorized)
			return
		}
		
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(authHandlerFunc)
}

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
		IdleConnTimeout: 90 * time.Second,
	}

	proxy := &httputil.ReverseProxy {
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.SetBasicAuth(user, pass)
		},
		Transport: customTransport,
		ErrorHandler: handleUpstreamError,
		ModifyResponse: logResponse,
	}

	return proxy
}


func main() {
	pcloudUser := "username"
	pcloudPass := "password"
	proxyUser := "testuser"
	proxyPass := "testpass"

	target, _ := url.Parse("https://127.0.0.1:8099")
	
	proxy := NewReverseProxy(pcloudUser, pcloudPass, target)
	handler := requireAuth(proxyUser, proxyPass, proxy)
	http.Handle("/", handler)
	
	log.Println("Begin listening")

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}

}
