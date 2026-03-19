package model

import (
	"time"

	"github.com/google/uuid"
)

// Label represents a tag/label that can be applied to library items.
// Table: omnivore.labels
type Label struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index:idx_labels_user_id"`
	Name        string    `gorm:"type:text;not null"`
	Color       string    `gorm:"type:text;not null;default:'#07D2D1'"`
	Description *string   `gorm:"type:text"`
	Position    int       `gorm:"type:integer;not null;default:0"`
	Internal    bool      `gorm:"type:boolean;not null;default:false"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User         *User         `gorm:"foreignKey:UserID"`
	LibraryItems []LibraryItem `gorm:"many2many:omnivore.entity_labels;foreignKey:ID;joinForeignKey:label_id;References:ID;joinReferences:library_item_id"`
}

func (Label) TableName() string {
	return "omnivore.labels"
}

// EntityLabel is the join table for many-to-many relationship between labels and library items.
// Table: omnivore.entity_labels
type EntityLabel struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	LibraryItemID uuid.UUID  `gorm:"column:library_item_id;type:uuid;not null;index:idx_entity_labels_library_item"`
	LabelID       uuid.UUID  `gorm:"column:label_id;type:uuid;not null;index:idx_entity_labels_label"`
	HighlightID   *uuid.UUID `gorm:"column:highlight_id;type:uuid;index:idx_entity_labels_highlight"`
	Source        string     `gorm:"column:source;type:text;not null;default:'user'"`

	// Relationships
	LibraryItem *LibraryItem `gorm:"foreignKey:LibraryItemID"`
	Label       *Label       `gorm:"foreignKey:LabelID"`
	Highlight   *Highlight   `gorm:"foreignKey:HighlightID"`
}

func (EntityLabel) TableName() string {
	return "omnivore.entity_labels"
}
