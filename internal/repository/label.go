package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/model"
	"gorm.io/gorm"
)

// LabelRepository handles label data access.
type LabelRepository struct {
	db *gorm.DB
}

// NewLabelRepository creates a new label repository.
func NewLabelRepository(database *gorm.DB) *LabelRepository {
	return &LabelRepository{db: database}
}

// GetByUserID retrieves all labels for a user.
func (r *LabelRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Label, error) {
	var labels []*model.Label
	
	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Where("user_id = ?", userID).
			Order("position ASC, name ASC").
			Find(&labels).Error
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to get labels: %w", err)
	}
	
	return labels, nil
}

// GetByID retrieves a label by ID.
func (r *LabelRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Label, error) {
	var label model.Label
	
	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.First(&label, "id = ?", id).Error
	})
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("label not found")
		}
		return nil, fmt.Errorf("failed to get label: %w", err)
	}
	
	return &label, nil
}

// FindByName retrieves a label by name (case-insensitive) for uniqueness checking.
func (r *LabelRepository) FindByName(ctx context.Context, userID uuid.UUID, name string) (*model.Label, error) {
	var label model.Label
	
	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Where("user_id = ? AND LOWER(name) = LOWER(?)", userID, name).
			First(&label).Error
	})
	
	if err == gorm.ErrRecordNotFound {
		return nil, nil // Not found is not an error for uniqueness check
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find label by name: %w", err)
	}
	
	return &label, nil
}

// Create creates a new label.
func (r *LabelRepository) Create(ctx context.Context, label *model.Label) error {
	return db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Create(label).Error
	})
}

// GetByIDs retrieves multiple labels by their IDs.
func (r *LabelRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*model.Label, error) {
	var labels []*model.Label
	
	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Where("id IN ?", ids).Find(&labels).Error
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to get labels by IDs: %w", err)
	}
	
	return labels, nil
}
