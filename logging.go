package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
)

type requestIDKey struct{}

// Tags each request with a random ID, returned to the client in X-Request-ID and
// included in every log line for that request
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 8)
		rand.Read(b)
		id := hex.EncodeToString(b)

		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

// Logs one line for a request. Only the method and path are logged, never headers, so
// credentials stay out of the logs. The path is quoted because it is client-controlled
// and may contain newlines that would otherwise forge extra log lines.
func logRequest(r *http.Request, msg string) {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	log.Printf("id=%s method=%s path=%q %s", id, r.Method, r.URL.Path, msg)
}
