package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/model"
	"github.com/omnivore-app/omnivore/internal/repository"
	"gorm.io/gorm"
)

// ArticleHandler handles article REST endpoints.
type ArticleHandler struct {
	db                   *gorm.DB
	libraryItemRepo      *repository.LibraryItemRepository
	labelRepo            *repository.LabelRepository
}

// NewArticleHandler creates a new article handler.
func NewArticleHandler(
	database *gorm.DB,
	libraryItemRepo *repository.LibraryItemRepository,
	labelRepo *repository.LabelRepository,
) *ArticleHandler {
	return &ArticleHandler{
		db:              database,
		libraryItemRepo: libraryItemRepo,
		labelRepo:       labelRepo,
	}
}

// ArticleResponse represents an article response.
type ArticleResponse struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	Author      string   `json:"author,omitempty"`
	Image       string   `json:"image,omitempty"`
	SavedAt     string   `json:"savedAt"`
	State       string   `json:"state"`
	Labels      []string `json:"labels"`
}

// UpdateArticleRequest represents an update request.
type UpdateArticleRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	State       *string `json:"state,omitempty"`
}

// GetArticle handles GET /api/article/:id
func (h *ArticleHandler) GetArticle(w http.ResponseWriter, r *http.Request) {
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
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/article/"), "/")
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

	// Fetch article using repository
	item, err := h.libraryItemRepo.GetByID(r.Context(), articleID)

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(w, "Article not found", http.StatusNotFound)
			return
		}
		respondError(w, "Failed to fetch article", http.StatusInternalServerError)
		return
	}

	// Check ownership
	if item.UserID != userID {
		respondError(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Extract label names from labels
	labelNames := make([]string, len(item.Labels))
	for i, label := range item.Labels {
		labelNames[i] = label.Name
	}

	// Convert to response
	title := ""
	if item.Title != nil {
		title = *item.Title
	}
	description := ""
	if item.Description != nil {
		description = *item.Description
	}
	author := ""
	if item.Author != nil {
		author = *item.Author
	}
	image := ""
	if item.Thumbnail != nil {
		image = *item.Thumbnail
	}

	resp := ArticleResponse{
		ID:          item.ID.String(),
		Title:       title,
		URL:         item.OriginalURL,
		Description: description,
		Author:      author,
		Image:       image,
		SavedAt:     item.SavedAt.Format("2006-01-02T15:04:05Z07:00"),
		State:       string(item.State),
		Labels:      labelNames,
	}

	respondJSON(w, resp)
}

// UpdateArticle handles PUT /api/article/:id
func (h *ArticleHandler) UpdateArticle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
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
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/article/"), "/")
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

	// Parse request body
	var req UpdateArticleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Update article using repository
	item, err := h.libraryItemRepo.GetByID(r.Context(), articleID)

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(w, "Article not found", http.StatusNotFound)
			return
		}
		respondError(w, "Failed to fetch article", http.StatusInternalServerError)
		return
	}

	// Check ownership
	if item.UserID != userID {
		respondError(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Apply updates
	if req.Title != nil {
		item.Title = req.Title
	}
	if req.Description != nil {
		item.Description = req.Description
	}
	if req.State != nil {
		item.State = model.LibraryItemState(*req.State)
	}

	// Update in DB
	err = h.libraryItemRepo.Update(r.Context(), item)
	if err != nil {
		respondError(w, "Failed to update article", http.StatusInternalServerError)
		return
	}

	// Extract label names from labels
	labelNames := make([]string, len(item.Labels))
	for i, label := range item.Labels {
		labelNames[i] = label.Name
	}

	// Convert to response
	title := ""
	if item.Title != nil {
		title = *item.Title
	}
	description := ""
	if item.Description != nil {
		description = *item.Description
	}
	author := ""
	if item.Author != nil {
		author = *item.Author
	}
	image := ""
	if item.Thumbnail != nil {
		image = *item.Thumbnail
	}

	resp := ArticleResponse{
		ID:          item.ID.String(),
		Title:       title,
		URL:         item.OriginalURL,
		Description: description,
		Author:      author,
		Image:       image,
		SavedAt:     item.SavedAt.Format("2006-01-02T15:04:05Z07:00"),
		State:       string(item.State),
		Labels:      labelNames,
	}

	respondJSON(w, resp)
}

// DeleteArticle handles DELETE /api/article/:id
func (h *ArticleHandler) DeleteArticle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
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
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/article/"), "/")
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

	// Delete article by setting state to DELETED
	item, err := h.libraryItemRepo.GetByID(r.Context(), articleID)

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(w, "Article not found", http.StatusNotFound)
			return
		}
		respondError(w, "Failed to fetch article", http.StatusInternalServerError)
		return
	}

	// Check ownership
	if item.UserID != userID {
		respondError(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Soft delete by setting state to DELETED
	item.State = model.LibraryItemStateDeleted
	err = h.libraryItemRepo.Update(r.Context(), item)
	if err != nil {
		respondError(w, "Failed to delete article", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]bool{"success": true})
}
