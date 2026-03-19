package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LibraryItemRepository handles library item data access.
type LibraryItemRepository struct {
	db *gorm.DB
}

// NewLibraryItemRepository creates a new library item repository.
func NewLibraryItemRepository(database *gorm.DB) *LibraryItemRepository {
	return &LibraryItemRepository{db: database}
}

// GetByID retrieves a library item by ID.
func (r *LibraryItemRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.LibraryItem, error) {
	var item model.LibraryItem

	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Preload("User").
			Preload("User.Profile").
			Preload("Labels").
			Preload("Highlights").
			Preload("Highlights.Labels").
			First(&item, "id = ?", id).Error
	})

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("library item not found")
		}
		return nil, fmt.Errorf("failed to get library item: %w", err)
	}

	return &item, nil
}

// GetBySlug retrieves a library item by slug.
func (r *LibraryItemRepository) GetBySlug(ctx context.Context, slug string) (*model.LibraryItem, error) {
	var item model.LibraryItem

	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Preload("User").
			Preload("User.Profile").
			Preload("Labels").
			Preload("Highlights").
			Preload("Highlights.Labels").
			Where("slug = ?", slug).
			First(&item).Error
	})

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("library item not found")
		}
		return nil, fmt.Errorf("failed to get library item: %w", err)
	}

	return &item, nil
}

// SetLabels replaces all labels for a library item.
func (r *LibraryItemRepository) SetLabels(ctx context.Context, itemID uuid.UUID, labelIDs []uuid.UUID) error {
	return db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		// Delete existing label assignments
		if err := tx.Where("library_item_id = ?", itemID).
			Delete(&model.EntityLabel{}).Error; err != nil {
			return fmt.Errorf("failed to delete existing labels: %w", err)
		}

		// Create new label assignments
		if len(labelIDs) > 0 {
			entityLabels := make([]model.EntityLabel, len(labelIDs))
			for i, labelID := range labelIDs {
				entityLabels[i] = model.EntityLabel{
					ID:            uuid.New(),
					LibraryItemID: itemID,
					LabelID:       labelID,
					Source:        "user",
				}
			}

			if err := tx.Create(&entityLabels).Error; err != nil {
				return fmt.Errorf("failed to create label assignments: %w", err)
			}
		}

		// Update denormalized label_names array
		if err := tx.Exec(`
UPDATE omnivore.library_item
SET label_names = COALESCE((
SELECT array_agg(DISTINCT l.name)
FROM omnivore.labels l
INNER JOIN omnivore.entity_labels el
ON el.label_id = l.id
AND el.library_item_id = $1
), ARRAY[]::TEXT[])
WHERE id = $1
`, itemID).Error; err != nil {
			return fmt.Errorf("failed to update label_names: %w", err)
		}

		return nil
	})
}

// Create creates a new library item.
func (r *LibraryItemRepository) Create(ctx context.Context, item *model.LibraryItem) error {
	return db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Create(item).Error
	})
}

// FindByURL finds a library item by original URL (using MD5 hash).
func (r *LibraryItemRepository) FindByURL(ctx context.Context, userID uuid.UUID, url string) (*model.LibraryItem, error) {
	var item model.LibraryItem

	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Where("user_id = ? AND md5(original_url) = md5(?)", userID, url).
			First(&item).Error
	})

	if err == gorm.ErrRecordNotFound {
		return nil, nil // Not found is not an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find library item by URL: %w", err)
	}

	return &item, nil
}

// Update updates an existing library item.
func (r *LibraryItemRepository) Update(ctx context.Context, item *model.LibraryItem) error {
	return db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Omit(clause.Associations).Save(item).Error
	})
}
