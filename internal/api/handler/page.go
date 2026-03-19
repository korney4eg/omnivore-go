package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/service"
)

// PageHandler handles page REST endpoints.
type PageHandler struct {
	saveURLService  *service.SaveURLService
	savePageService *service.SavePageService
}

// NewPageHandler creates a new page handler.
func NewPageHandler(
	saveURLService *service.SaveURLService,
	savePageService *service.SavePageService,
) *PageHandler {
	return &PageHandler{
		saveURLService:  saveURLService,
		savePageService: savePageService,
	}
}

// SavePageRequest represents a quick save request from browser extension.
type SavePageRequest struct {
	URL          string  `json:"url"`
	OriginalHTML *string `json:"originalHtml,omitempty"`
	Title        *string `json:"title,omitempty"`
	Source       string  `json:"source,omitempty"`
}

// SavePageResponse represents a save response.
type SavePageResponse struct {
	URL             string `json:"url"`
	ClientRequestID string `json:"clientRequestId"`
	ID              string `json:"id"`
}

// Save handles POST /api/page/save
// Used by browser extension for quick saving
func (h *PageHandler) Save(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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

	var req SavePageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		respondError(w, "URL is required", http.StatusBadRequest)
		return
	}

	source := req.Source
	if source == "" {
		source = "browser-extension"
	}

	// If HTML provided, use savePage; otherwise saveUrl
	if req.OriginalHTML != nil && *req.OriginalHTML != "" {
		// Save with HTML
		result, err := h.savePageService.SavePage(r.Context(), service.SavePageInput{
			UserID:       userID,
			URL:          req.URL,
			OriginalHTML: *req.OriginalHTML,
			Title:        req.Title,
			Source:       source,
		})

		if err != nil {
			respondError(w, "Failed to save page: "+err.Error(), http.StatusInternalServerError)
			return
		}

		respondJSON(w, SavePageResponse{
			URL:             result.URL,
			ClientRequestID: result.ClientRequestID,
			ID:              result.LibraryItemID.String(),
		})
	} else {
		// Save URL only
		result, err := h.saveURLService.SaveURL(r.Context(), service.SaveURLInput{
			UserID:          userID,
			URL:             req.URL,
			Source:          source,
			ClientRequestID: uuid.New().String(),
		})

		if err != nil {
			respondError(w, "Failed to save URL: "+err.Error(), http.StatusInternalServerError)
			return
		}

		respondJSON(w, SavePageResponse{
			URL:             result.URL,
			ClientRequestID: result.ClientRequestID,
			ID:              result.LibraryItemID.String(),
		})
	}
}
