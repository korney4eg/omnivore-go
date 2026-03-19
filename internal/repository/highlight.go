package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/model"
	"gorm.io/gorm"
)

// HighlightRepository handles highlight data access.
type HighlightRepository struct {
	db *gorm.DB
}

// NewHighlightRepository creates a new highlight repository.
func NewHighlightRepository(database *gorm.DB) *HighlightRepository {
	return &HighlightRepository{db: database}
}

// GetByLibraryItemID retrieves highlights for a library item.
func (r *HighlightRepository) GetByLibraryItemID(ctx context.Context, libraryItemID uuid.UUID) ([]*model.Highlight, error) {
	var highlights []*model.Highlight

	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Where("library_item_id = ?", libraryItemID).
			Where("deleted_at IS NULL").
			Preload("Labels").
			Order("created_at ASC").
			Find(&highlights).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get highlights: %w", err)
	}

	return highlights, nil
}

// GetByUserID retrieves all highlights for a user.
func (r *HighlightRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Highlight, error) {
	var highlights []*model.Highlight

	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Where("user_id = ?", userID).
			Where("deleted_at IS NULL").
			Preload("Labels").
			Order("created_at DESC").
			Find(&highlights).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get highlights: %w", err)
	}

	return highlights, nil
}

// Create creates a new highlight.
func (r *HighlightRepository) Create(ctx context.Context, highlight *model.Highlight) error {
	return db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Create(highlight).Error
	})
}

// Update updates an existing highlight.
func (r *HighlightRepository) Update(ctx context.Context, highlight *model.Highlight) error {
	return db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Save(highlight).Error
	})
}

// Delete deletes a highlight by ID.
func (r *HighlightRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Delete(&model.Highlight{}, "id = ?", id).Error
	})
}

// GetByID retrieves a single highlight by ID.
func (r *HighlightRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Highlight, error) {
	var highlight model.Highlight

	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Where("id = ?", id).First(&highlight).Error
	})

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("highlight not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get highlight: %w", err)
	}

	return &highlight, nil
}
