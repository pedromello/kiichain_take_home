package middleware

import "net/http"

// HeadersMiddleware adds custom response headers required by the specification
// (such as X-Empty-Header) to all HTTP responses.
func HeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set the required empty header in the response
		w.Header().Set("X-Empty-Header", "")
		next.ServeHTTP(w, r)
	})
}
