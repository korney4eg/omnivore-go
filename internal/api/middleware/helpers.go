package middleware

import (
"net/http"
"strings"
)

// ApplyToPrefix applies middleware only to paths starting with the given prefix.
func ApplyToPrefix(prefix string, middleware func(http.Handler) http.Handler, handler http.Handler) http.Handler {
return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if strings.HasPrefix(r.URL.Path, prefix) {
middleware(handler).ServeHTTP(w, r)
} else {
handler.ServeHTTP(w, r)
}
})
}
