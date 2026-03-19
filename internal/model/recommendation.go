package model

import (
	"time"

	"github.com/google/uuid"
)

// RecommendationType represents the type of recommendation.
type RecommendationType string

const (
	RecommendationTypeLibraryItem RecommendationType = "LIBRARY_ITEM"
	RecommendationTypeFeed        RecommendationType = "FEED"
)

// Recommendation represents a content recommendation.
// Table: omnivore.recommendations
type Recommendation struct {
	ID            uuid.UUID          `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID        uuid.UUID          `gorm:"column:user_id;type:uuid;not null;index:idx_recommendations_user_id"`
	LibraryItemID *uuid.UUID         `gorm:"column:library_item_id;type:uuid;index:idx_recommendations_library_item"`
	RecommenderID uuid.UUID          `gorm:"column:recommender_id;type:uuid;not null"`
	Name          *string            `gorm:"type:text"`
	Note          *string            `gorm:"type:text"`
	Type          RecommendationType `gorm:"type:recommendation_type;not null"`
	CreatedAt     time.Time          `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User        *User        `gorm:"foreignKey:UserID"`
	LibraryItem *LibraryItem `gorm:"foreignKey:LibraryItemID"`
	Recommender *User        `gorm:"foreignKey:RecommenderID"`
}

func (Recommendation) TableName() string {
	return "omnivore.recommendations"
}

// Reminder represents a reminder for a library item.
// Table: omnivore.reminders
type Reminder struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID        uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_reminders_user_id"`
	LibraryItemID uuid.UUID  `gorm:"column:library_item_id;type:uuid;not null;index:idx_reminders_library_item"`
	RemindAt      time.Time  `gorm:"column:remind_at;type:timestamptz;not null"`
	Status        string     `gorm:"type:text;not null;default:'PENDING'"` // PENDING, SENT, CANCELLED
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User        *User        `gorm:"foreignKey:UserID"`
	LibraryItem *LibraryItem `gorm:"foreignKey:LibraryItemID"`
}

func (Reminder) TableName() string {
	return "omnivore.reminders"
}

// Reaction represents a user's reaction to a library item.
// Table: omnivore.reactions
type Reaction struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID        uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_reactions_user_id"`
	LibraryItemID uuid.UUID  `gorm:"column:library_item_id;type:uuid;not null;index:idx_reactions_library_item"`
	Code          string     `gorm:"type:text;not null"` // Reaction emoji code
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User        *User        `gorm:"foreignKey:UserID"`
	LibraryItem *LibraryItem `gorm:"foreignKey:LibraryItemID"`
}

func (Reaction) TableName() string {
	return "omnivore.reactions"
}
