package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

// Handler on authentication from Zotero WebDAV
func requireAuth(user, pass string, next http.Handler) http.Handler {
	// ConstantTimeCompare returns early when lengths differ, so compare fixed-length
	// digests instead of the raw values to avoid leaking the credential lengths
	wantUser := sha256.Sum256([]byte(user))
	wantPass := sha256.Sum256([]byte(pass))

	authHandlerFunc := func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		gotUser := sha256.Sum256([]byte(u))
		gotPass := sha256.Sum256([]byte(p))
		userIsValid := subtle.ConstantTimeCompare(gotUser[:], wantUser[:]) == 1
		passIsValid := subtle.ConstantTimeCompare(gotPass[:], wantPass[:]) == 1

		if !ok || !userIsValid || !passIsValid {
			w.Header().Set("WWW-Authenticate", `Basic realm="zotero-webdav"`)
			logRequest(r, "status=unauthorised-from-zotero")
			http.Error(w, "Unauthorised", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(authHandlerFunc)
}
