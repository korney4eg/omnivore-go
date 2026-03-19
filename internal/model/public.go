package model

import (
	"time"

	"github.com/google/uuid"
)

// PublicItem represents publicly shared content.
// Table: omnivore.public_items
type PublicItem struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	LibraryItemID uuid.UUID  `gorm:"column:library_item_id;type:uuid;not null;uniqueIndex"`
	Slug          string     `gorm:"type:text;not null;uniqueIndex"`
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	LibraryItem *LibraryItem `gorm:"foreignKey:LibraryItemID"`
}

func (PublicItem) TableName() string {
	return "omnivore.public_items"
}

// PublicItemSource represents the source of a public item share.
// Table: omnivore.public_item_sources
type PublicItemSource struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	PublicItemID uuid.UUID  `gorm:"column:public_item_id;type:uuid;not null;index:idx_public_item_sources_public_item"`
	UserID       uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_public_item_sources_user"`
	Type         string     `gorm:"type:text;not null"` // e.g., "SHARE", "RECOMMENDATION"
	CreatedAt    time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	PublicItem *PublicItem `gorm:"foreignKey:PublicItemID"`
	User       *User       `gorm:"foreignKey:UserID"`
}

func (PublicItemSource) TableName() string {
	return "omnivore.public_item_sources"
}

// PublicItemInteraction represents interactions with public items (views, saves).
// Table: omnivore.public_item_interactions
type PublicItemInteraction struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	PublicItemID uuid.UUID  `gorm:"column:public_item_id;type:uuid;not null;index:idx_public_item_interactions_public_item"`
	UserID       *uuid.UUID `gorm:"column:user_id;type:uuid;index:idx_public_item_interactions_user"`
	Type         string     `gorm:"type:text;not null"` // VIEW, SAVE, etc.
	CreatedAt    time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	PublicItem *PublicItem `gorm:"foreignKey:PublicItemID"`
	User       *User       `gorm:"foreignKey:UserID"`
}

func (PublicItemInteraction) TableName() string {
	return "omnivore.public_item_interactions"
}

// PublicItemStats represents aggregated statistics for public items.
// Table: omnivore.public_item_stats
type PublicItemStats struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	PublicItemID uuid.UUID  `gorm:"column:public_item_id;type:uuid;not null;uniqueIndex"`
	ViewCount    int        `gorm:"column:view_count;type:integer;not null;default:0"`
	SaveCount    int        `gorm:"column:save_count;type:integer;not null;default:0"`
	UpdatedAt    time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	PublicItem *PublicItem `gorm:"foreignKey:PublicItemID"`
}

func (PublicItemStats) TableName() string {
	return "omnivore.public_item_stats"
}
