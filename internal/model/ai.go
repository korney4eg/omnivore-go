package model

import (
	"time"

	"github.com/google/uuid"
)

// AISummary represents an AI-generated summary of a library item.
// Table: omnivore.ai_summaries
type AISummary struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID        uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_ai_summaries_user_id"`
	LibraryItemID uuid.UUID  `gorm:"column:library_item_id;type:uuid;not null;index:idx_ai_summaries_library_item"`
	Summary       string     `gorm:"type:text;not null"`
	Model         string     `gorm:"type:text;not null"` // e.g., "gpt-4", "claude-3"
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User        *User        `gorm:"foreignKey:UserID"`
	LibraryItem *LibraryItem `gorm:"foreignKey:LibraryItemID"`
}

func (AISummary) TableName() string {
	return "omnivore.ai_summaries"
}

// Speech represents a text-to-speech audio file.
// Table: omnivore.speeches
type Speech struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID        uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_speeches_user_id"`
	LibraryItemID uuid.UUID  `gorm:"column:library_item_id;type:uuid;not null;index:idx_speeches_library_item"`
	AudioURL      string     `gorm:"column:audio_url;type:text;not null"`
	Voice         string     `gorm:"type:text;not null"`
	Language      string     `gorm:"type:text;not null"`
	State         string     `gorm:"type:text;not null;default:'PENDING'"` // PENDING, PROCESSING, SUCCEEDED, FAILED
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User        *User        `gorm:"foreignKey:UserID"`
	LibraryItem *LibraryItem `gorm:"foreignKey:LibraryItemID"`
}

func (Speech) TableName() string {
	return "omnivore.speeches"
}
