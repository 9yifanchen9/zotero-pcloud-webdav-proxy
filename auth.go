package main

import (
	"crypto/subtle"
	"log"
	"net/http"
)

// Handler on authentication from Zotero WebDAV
func requireAuth(user, pass string, next http.Handler) http.Handler {
	authHandlerFunc := func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		userIsValid := subtle.ConstantTimeCompare([]byte(u), []byte(user)) == 1
		passIsValid := subtle.ConstantTimeCompare([]byte(p), []byte(pass)) == 1

		if !ok || !userIsValid || !passIsValid {
			w.Header().Set("WWW-Authenticate", `Basic realm="zotero-webdav"`)
			log.Printf("method=%s path=%s status=unauthorised-from-zotero", r.Method, r.URL.Path)
			http.Error(w, "Unauthorised", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(authHandlerFunc)
}
