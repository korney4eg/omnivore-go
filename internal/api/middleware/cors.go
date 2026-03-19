package middleware

import (
"net/http"
)

// CORS adds Cross-Origin Resource Sharing headers.
func CORS(next http.Handler) http.Handler {
return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
origin := r.Header.Get("Origin")

if origin != "" {
w.Header().Set("Access-Control-Allow-Origin", origin)
w.Header().Set("Access-Control-Allow-Credentials", "true")
}

if r.Method == http.MethodOptions {
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-OmnivoreClient")
w.Header().Set("Access-Control-Max-Age", "86400")
w.WriteHeader(http.StatusNoContent)
return
}

next.ServeHTTP(w, r)
})
}
