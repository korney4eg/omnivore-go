package middleware

import (
	"net/http"
	"strings"

	"github.com/omnivore-app/omnivore/internal/auth"
	"github.com/omnivore-app/omnivore/internal/db"
)

// AuthMiddleware holds auth configuration.
type AuthMiddleware struct {
	jwtConfig    *auth.JWTConfig
	apiKeyConfig *auth.APIKeyConfig
}

// NewAuthMiddleware creates a new auth middleware with config.
func NewAuthMiddleware(jwtConfig *auth.JWTConfig, apiKeyConfig *auth.APIKeyConfig) *AuthMiddleware {
	return &AuthMiddleware{
		jwtConfig:    jwtConfig,
		apiKeyConfig: apiKeyConfig,
	}
}

// Auth extracts JWT or API key from request and sets user context.
func (a *AuthMiddleware) Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Try API key first (X-API-Key header)
		if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
			if a.apiKeyConfig != nil {
				info, err := a.apiKeyConfig.ValidateAPIKey(ctx, apiKey)
				if err == nil {
					// Set user context from API key
					ctx = db.SetUserContext(ctx, &db.User{
						ID:   info.UserID.String(),
						Role: info.UserRole,
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// Try JWT from Authorization header
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if a.jwtConfig != nil {
				claims, err := a.jwtConfig.ValidateToken(token)
				if err == nil {
					// Set user context from JWT
					ctx = db.SetUserContext(ctx, &db.User{
						ID:   claims.UserID,
						Role: claims.UserRole,
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// Try JWT from cookie
		if cookie, err := r.Cookie("auth"); err == nil && cookie.Value != "" {
			if a.jwtConfig != nil {
				claims, err := a.jwtConfig.ValidateToken(cookie.Value)
				if err == nil {
					// Set user context from JWT
					ctx = db.SetUserContext(ctx, &db.User{
						ID:   claims.UserID,
						Role: claims.UserRole,
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// No valid authentication found, continue without user context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth is middleware that requires authentication.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasUser := db.GetUserFromContext(r.Context())
		if !hasUser {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
