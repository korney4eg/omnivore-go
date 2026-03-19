package handler

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/repository"
	"gorm.io/gorm"
)

// ContentHandler handles content REST endpoints.
type ContentHandler struct {
	db              *gorm.DB
	libraryItemRepo *repository.LibraryItemRepository
}

// NewContentHandler creates a new content handler.
func NewContentHandler(
	database *gorm.DB,
	libraryItemRepo *repository.LibraryItemRepository,
) *ContentHandler {
	return &ContentHandler{
		db:              database,
		libraryItemRepo: libraryItemRepo,
	}
}

// GetContent handles GET /api/content/:id
// Returns HTML content for an article
func (h *ContentHandler) GetContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user
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

	// Extract article ID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/content/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		respondError(w, "Article ID required", http.StatusBadRequest)
		return
	}

	articleIDStr := pathParts[0]
	articleID, err := uuid.Parse(articleIDStr)
	if err != nil {
		respondError(w, "Invalid article ID", http.StatusBadRequest)
		return
	}

	// Fetch article
	item, err := h.libraryItemRepo.GetByID(r.Context(), articleID)

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(w, "Article not found", http.StatusNotFound)
			return
		}
		respondError(w, "Failed to fetch content", http.StatusInternalServerError)
		return
	}

	// Check ownership
	if item.UserID != userID {
		respondError(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Get readable content
	content := ""
	if item.ReadableContent != nil {
		content = *item.ReadableContent
	}

	// Return HTML content
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(content))
}
