package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/model"
	"gorm.io/gorm"
)

type SubscriptionRepository struct {
	db *gorm.DB
}

func NewSubscriptionRepository(database *gorm.DB) *SubscriptionRepository {
	return &SubscriptionRepository{db: database}
}

// FindByUserID returns subscriptions for a user with optional type filter and sorting.
func (r *SubscriptionRepository) FindByUserID(
	ctx context.Context,
	userID uuid.UUID,
	subscriptionType *model.SubscriptionType,
	sortBy string,
	sortOrder string,
) ([]model.Subscription, error) {
	var subscriptions []model.Subscription

	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		query := tx.Preload("NewsletterEmail").Where("user_id = ?", userID)

		// Type filtering with special status handling
		if subscriptionType != nil {
			switch *subscriptionType {
			case model.SubscriptionTypeNewsletter:
				// Only ACTIVE newsletters
				query = query.Where("type = ? AND status = ?", model.SubscriptionTypeNewsletter, model.SubscriptionStatusActive)
			case model.SubscriptionTypeRSS:
				// All RSS subscriptions regardless of status
				query = query.Where("type = ?", model.SubscriptionTypeRSS)
			}
		} else {
			// Default: ACTIVE newsletters OR all RSS
			query = query.Where(
				tx.Where("type = ? AND status = ?", model.SubscriptionTypeNewsletter, model.SubscriptionStatusActive).
					Or("type = ?", model.SubscriptionTypeRSS),
			)
		}

		// Primary sort: status ASC (ACTIVE first)
		query = query.Order("status ASC")

		// Secondary sort: by specified field
		if sortBy == "" {
			sortBy = "created_at"
		}
		if sortOrder == "" {
			sortOrder = "DESC"
		}

		// Add NULLS LAST for timestamp fields
		if sortBy == "refreshed_at" || sortBy == "created_at" || sortBy == "updated_at" {
			query = query.Order(fmt.Sprintf("%s %s NULLS LAST", sortBy, sortOrder))
		} else {
			query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))
		}

		return query.Find(&subscriptions).Error
	})

	return subscriptions, err
}

// FindByID returns a subscription by ID.
func (r *SubscriptionRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Subscription, error) {
	var subscription model.Subscription

	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Preload("NewsletterEmail").Where("id = ?", id).First(&subscription).Error
	})

	if err != nil {
		return nil, err
	}
	return &subscription, nil
}

// FindRSSByUserAndURLs returns the first RSS subscription matching any of the URLs for the user.
func (r *SubscriptionRepository) FindRSSByUserAndURLs(ctx context.Context, userID uuid.UUID, urls []string) (*model.Subscription, error) {
	var subscription model.Subscription

	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.
			Where("user_id = ? AND type = ? AND url IN ?", userID, model.SubscriptionTypeRSS, urls).
			First(&subscription).Error
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &subscription, nil
}

// CountActiveRSSByUser returns the number of active RSS subscriptions for the user.
func (r *SubscriptionRepository) CountActiveRSSByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64

	err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.
			Model(&model.Subscription{}).
			Where("user_id = ? AND type = ? AND status = ?", userID, model.SubscriptionTypeRSS, model.SubscriptionStatusActive).
			Count(&count).Error
	})

	return count, err
}

// Save persists a subscription using the current auth context.
func (r *SubscriptionRepository) Save(ctx context.Context, subscription *model.Subscription) error {
	return db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Save(subscription).Error
	})
}
