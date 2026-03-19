package model

import (
	"time"

	"github.com/google/uuid"
)

// HighlightType represents the type of highlight.
type HighlightType string

const (
	HighlightTypeHighlight HighlightType = "HIGHLIGHT"
	HighlightTypeNote      HighlightType = "NOTE"
	HighlightTypeRedaction HighlightType = "REDACTION"
)

// Highlight represents a user's highlight or annotation on a library item.
// Table: omnivore.highlight
type Highlight struct {
	ID                           uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID                       uuid.UUID      `gorm:"column:user_id;type:uuid;not null;index:idx_highlights_user_id"`
	LibraryItemID                uuid.UUID      `gorm:"column:library_item_id;type:uuid;not null;index:idx_highlights_library_item"`
	ShortID                      string         `gorm:"column:short_id;type:text;not null;uniqueIndex"`
	Quote                        *string        `gorm:"type:text"`
	Prefix                       *string        `gorm:"type:text"`
	Suffix                       *string        `gorm:"type:text"`
	Patch                        *string        `gorm:"type:text"`
	Annotation                   *string        `gorm:"type:text"`
	HighlightType                *HighlightType `gorm:"column:highlight_type;type:highlight_type"`
	Color                        *string        `gorm:"type:text"`
	HighlightPositionPercent     *float64       `gorm:"column:highlight_position_percent;type:real"`
	HighlightPositionAnchorIndex *int           `gorm:"column:highlight_position_anchor_index;type:integer"`
	SharedAt                     *time.Time     `gorm:"column:shared_at;type:timestamptz"`
	HTML                         *string        `gorm:"type:text"`
	CreatedAt                    time.Time      `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt                    time.Time      `gorm:"type:timestamptz;not null;default:current_timestamp"`
	DeletedAt                    *time.Time     `gorm:"column:deleted_at;type:timestamptz"`

	// Relationships
	User        *User        `gorm:"foreignKey:UserID"`
	LibraryItem *LibraryItem `gorm:"foreignKey:LibraryItemID"`
	Labels      []Label      `gorm:"many2many:omnivore.entity_labels;foreignKey:ID;joinForeignKey:highlight_id;References:ID;joinReferences:label_id"`
}

func (Highlight) TableName() string {
	return "omnivore.highlight"
}
