package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/auth"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/model"
	"github.com/omnivore-app/omnivore/internal/service"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthHandler handles authentication REST endpoints.
type AuthHandler struct {
	db        *gorm.DB
	jwtConfig *auth.JWTConfig
	userSvc   *service.UserService
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(database *gorm.DB, jwtConfig *auth.JWTConfig) *AuthHandler {
	return &AuthHandler{
		db:        database,
		jwtConfig: jwtConfig,
		userSvc:   service.NewUserService(database),
	}
}

// LoginRequest represents a login request.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// SignupRequest represents a signup request.
type SignupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

// AuthResponse represents an auth response.
type AuthResponse struct {
	User  UserResponse `json:"user"`
	Token string       `json:"token"`
}

// UserResponse represents a user in response.
type UserResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

type verifyResponse struct {
	AuthStatus string `json:"authStatus"`
}

const (
	authStatusAuthenticated       = "AUTHENTICATED"
	authStatusNotAuthenticated    = "NOT_AUTHENTICATED"
	defaultHomePath               = "/home"
	emailLoginPath                = "/auth/email-login"
	loginErrorInvalidCredentials  = "INVALID_CREDENTIALS"
	loginErrorUserNotFound        = "USER_NOT_FOUND"
	loginErrorWrongSource         = "WRONG_SOURCE"
	loginErrorAuthFailed          = "AUTH_FAILED"
	loginErrorPendingVerification = "PENDING_VERIFICATION"
)

// Login handles POST /api/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	user, loginErr := h.authenticateUser(r, req.Email, req.Password)
	if loginErr != nil {
		respondError(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := h.issueSession(w, r, user)
	if err != nil {
		respondError(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Return response
	username := ""
	if user.Profile != nil {
		username = user.Profile.Username
	}

	respondJSON(w, AuthResponse{
		User: UserResponse{
			ID:       user.ID.String(),
			Email:    user.Email,
			Name:     user.Name,
			Username: username,
		},
		Token: token,
	})
}

// Signup handles POST /api/auth/signup
func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate input
	if req.Email == "" || req.Password == "" || req.Name == "" || req.Username == "" {
		respondError(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	result, err := h.userSvc.CreateEmailUser(r.Context(), service.CreateEmailUserInput{
		Email:    req.Email,
		Password: req.Password,
		Name:     req.Name,
		Username: req.Username,
	})
	if err != nil {
		switch {
		case err == service.ErrUserAlreadyExists:
			respondError(w, "User already exists", http.StatusConflict)
			return
		case err == service.ErrUsernameTaken:
			respondError(w, "Username already exists", http.StatusConflict)
			return
		default:
			respondError(w, "Failed to create user", http.StatusInternalServerError)
			return
		}
	}

	// Generate JWT token
	token, err := h.jwtConfig.GenerateToken(result.UserID, "omnivore_user")
	if err != nil {
		respondError(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60, // 7 days
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Return response
	respondJSON(w, AuthResponse{
		User: UserResponse{
			ID:       result.UserID.String(),
			Email:    result.Email,
			Name:     result.Name,
			Username: result.Username,
		},
		Token: token,
	})
}

// Logout handles POST /api/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clear auth cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	respondJSON(w, map[string]bool{"success": true})
}

// Verify handles GET /api/auth/verify
func (h *AuthHandler) Verify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if _, hasUser := db.GetUserFromContext(r.Context()); hasUser {
		respondJSON(w, verifyResponse{AuthStatus: authStatusAuthenticated})
		return
	}

	respondJSON(w, verifyResponse{AuthStatus: authStatusNotAuthenticated})
}

// EmailLogin handles POST /api/auth/email-login
func (h *AuthHandler) EmailLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectLoginError(w, r, loginErrorAuthFailed)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	if email == "" || len(password) < 8 {
		h.redirectLoginError(w, r, loginErrorInvalidCredentials)
		return
	}

	user, loginErr := h.authenticateUser(r, email, password)
	if loginErr != nil {
		h.redirectLoginError(w, r, loginErr.Error())
		return
	}

	if _, err := h.issueSession(w, r, user); err != nil {
		h.redirectLoginError(w, r, loginErrorAuthFailed)
		return
	}

	http.Redirect(w, r, defaultHomePath, http.StatusFound)
}

// Me handles GET /api/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user from context
	user, hasUser := db.GetUserFromContext(r.Context())
	if !hasUser {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	userID, err := uuid.Parse(user.ID)
	if err != nil {
		respondError(w, "Invalid user ID", http.StatusInternalServerError)
		return
	}

	// Fetch full user with profile
	var dbUser model.User
	err = h.db.WithContext(r.Context()).
		Preload("Profile").
		Where("id = ?", userID).
		First(&dbUser).Error

	if err != nil {
		respondError(w, "User not found", http.StatusNotFound)
		return
	}

	username := ""
	if dbUser.Profile != nil {
		username = dbUser.Profile.Username
	}

	respondJSON(w, UserResponse{
		ID:       dbUser.ID.String(),
		Email:    dbUser.Email,
		Name:     dbUser.Name,
		Username: username,
	})
}

func (h *AuthHandler) authenticateUser(r *http.Request, email, password string) (*model.User, error) {
	var user model.User
	err := h.db.WithContext(r.Context()).
		Preload("Profile").
		Where("email = ?", strings.TrimSpace(email)).
		First(&user).Error

	if err == gorm.ErrRecordNotFound {
		return nil, errorWithMessage(loginErrorUserNotFound)
	}
	if err != nil {
		return nil, errorWithMessage(loginErrorAuthFailed)
	}
	if user.Status == model.UserStatusPending {
		return nil, errorWithMessage(loginErrorPendingVerification)
	}
	if user.Password == nil {
		return nil, errorWithMessage(loginErrorWrongSource)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.Password), []byte(password)); err != nil {
		return nil, errorWithMessage(loginErrorInvalidCredentials)
	}

	return &user, nil
}

func (h *AuthHandler) issueSession(w http.ResponseWriter, r *http.Request, user *model.User) (string, error) {
	token, err := h.jwtConfig.GenerateToken(user.ID, "omnivore_user")
	if err != nil {
		return "", err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	return token, nil
}

func (h *AuthHandler) redirectLoginError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, emailLoginPath+"?errorCodes="+code, http.StatusFound)
}

type errorWithMessage string

func (e errorWithMessage) Error() string {
	return string(e)
}

// respondJSON writes a JSON response.
func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// respondError writes an error response.
func respondError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error:   http.StatusText(statusCode),
		Message: message,
	})
}
