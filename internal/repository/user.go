package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/model"
	"gorm.io/gorm"
)

// UserRepository handles user data access.
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a new user repository.
func NewUserRepository(database *gorm.DB) *UserRepository {
	return &UserRepository{db: database}
}

// GetByID retrieves a user by ID.
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var user model.User
	
	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Preload("Profile").
			Preload("UserPersonalization").
			First(&user, "id = ?", id).Error
	})
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	
	return &user, nil
}

// GetByEmail retrieves a user by email.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	
	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Preload("Profile").
			Preload("UserPersonalization").
			Where("email = ?", email).
			First(&user).Error
	})
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	
	return &user, nil
}
