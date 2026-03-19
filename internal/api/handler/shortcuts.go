package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/model"
	"gorm.io/gorm"
)

type ShortcutsHandler struct {
	db *gorm.DB
}

func NewShortcutsHandler(database *gorm.DB) *ShortcutsHandler {
	return &ShortcutsHandler{db: database}
}

type shortcutsRequest struct {
	Shortcuts json.RawMessage `json:"shortcuts"`
}

type shortcutsResponse struct {
	Shortcuts json.RawMessage `json:"shortcuts"`
}

func (h *ShortcutsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.get(w, r)
	case http.MethodPut:
		h.put(w, r)
	case http.MethodDelete:
		h.delete(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ShortcutsHandler) get(w http.ResponseWriter, r *http.Request) {
	personalization, ok := h.loadPersonalization(w, r)
	if !ok {
		return
	}

	respondJSON(w, shortcutsResponse{Shortcuts: shortcutsPayload(personalization)})
}

func (h *ShortcutsHandler) put(w http.ResponseWriter, r *http.Request) {
	user, hasUser := db.GetUserFromContext(r.Context())
	if !hasUser {
		http.Error(w, "UNAUTHORIZED", http.StatusUnauthorized)
		return
	}

	var req shortcutsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "BAD_REQUEST", http.StatusBadRequest)
		return
	}

	if len(req.Shortcuts) == 0 {
		req.Shortcuts = json.RawMessage("[]")
	}

	var decoded []any
	if err := json.Unmarshal(req.Shortcuts, &decoded); err != nil {
		http.Error(w, "BAD_REQUEST", http.StatusBadRequest)
		return
	}

	userID, err := uuid.Parse(user.ID)
	if err != nil {
		http.Error(w, "UNKNOWN", http.StatusInternalServerError)
		return
	}

	payload := string(req.Shortcuts)
	err = h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		var personalization model.UserPersonalization
		findErr := tx.Where("user_id = ?", userID).First(&personalization).Error
		if findErr == nil {
			personalization.Shortcuts = &payload
			return tx.Save(&personalization).Error
		}
		if findErr != nil && findErr != gorm.ErrRecordNotFound {
			return findErr
		}

		personalization = model.UserPersonalization{
			ID:        uuid.New(),
			UserID:    userID,
			Shortcuts: &payload,
		}
		return tx.Create(&personalization).Error
	})
	if err != nil {
		http.Error(w, "UNKNOWN", http.StatusInternalServerError)
		return
	}

	respondJSON(w, shortcutsResponse{Shortcuts: req.Shortcuts})
}

func (h *ShortcutsHandler) delete(w http.ResponseWriter, r *http.Request) {
	user, hasUser := db.GetUserFromContext(r.Context())
	if !hasUser {
		http.Error(w, "UNAUTHORIZED", http.StatusUnauthorized)
		return
	}

	userID, err := uuid.Parse(user.ID)
	if err != nil {
		http.Error(w, "UNKNOWN", http.StatusInternalServerError)
		return
	}

	if err := h.db.WithContext(r.Context()).
		Model(&model.UserPersonalization{}).
		Where("user_id = ?", userID).
		Update("shortcuts", nil).Error; err != nil {
		http.Error(w, "UNKNOWN", http.StatusInternalServerError)
		return
	}

	respondJSON(w, shortcutsResponse{Shortcuts: json.RawMessage("[]")})
}

func (h *ShortcutsHandler) loadPersonalization(w http.ResponseWriter, r *http.Request) (*model.UserPersonalization, bool) {
	user, hasUser := db.GetUserFromContext(r.Context())
	if !hasUser {
		http.Error(w, "UNAUTHORIZED", http.StatusUnauthorized)
		return nil, false
	}

	userID, err := uuid.Parse(user.ID)
	if err != nil {
		http.Error(w, "UNKNOWN", http.StatusInternalServerError)
		return nil, false
	}

	var personalization model.UserPersonalization
	err = h.db.WithContext(r.Context()).
		Where("user_id = ?", userID).
		First(&personalization).Error
	if err == gorm.ErrRecordNotFound {
		return &model.UserPersonalization{}, true
	}
	if err != nil {
		http.Error(w, "UNKNOWN", http.StatusInternalServerError)
		return nil, false
	}

	return &personalization, true
}

func shortcutsPayload(personalization *model.UserPersonalization) json.RawMessage {
	if personalization == nil || personalization.Shortcuts == nil || *personalization.Shortcuts == "" {
		return json.RawMessage("[]")
	}

	return json.RawMessage(*personalization.Shortcuts)
}
