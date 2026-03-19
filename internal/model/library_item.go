package model

import (
	"time"

	"github.com/google/uuid"
)

// LibraryItemState represents the state of a library item.
type LibraryItemState string

const (
	LibraryItemStateSucceeded  LibraryItemState = "SUCCEEDED"
	LibraryItemStateProcessing LibraryItemState = "PROCESSING"
	LibraryItemStateFailed     LibraryItemState = "FAILED"
	LibraryItemStateDeleted    LibraryItemState = "DELETED"
	LibraryItemStateArchived   LibraryItemState = "ARCHIVED"
)

// ContentReader represents the reader type for content.
type ContentReader string

const (
	ContentReaderWeb  ContentReader = "WEB"
	ContentReaderPDF  ContentReader = "PDF"
	ContentReaderEPUB ContentReader = "EPUB"
)

// LibraryItem represents a saved article/page.
// This is the core entity for saved content in Omnivore.
// Table: omnivore.library_item
type LibraryItem struct {
	ID                         uuid.UUID        `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID                     uuid.UUID        `gorm:"type:uuid;not null;index:idx_library_item_user_id"`
	Slug                       string           `gorm:"type:text;not null;uniqueIndex"`
	OriginalURL                string           `gorm:"column:original_url;type:text;not null"`
	OriginalHTML               *string          `gorm:"-"`
	ReadableContent            *string          `gorm:"column:readable_content;type:text"`
	Title                      *string          `gorm:"type:text"`
	Author                     *string          `gorm:"type:text"`
	Description                *string          `gorm:"type:text"`
	Thumbnail                  *string          `gorm:"type:text"`
	PublishedAt                *time.Time       `gorm:"column:published_at;type:timestamptz"`
	SavedAt                    time.Time        `gorm:"column:saved_at;type:timestamptz;not null;default:current_timestamp"`
	ReadAt                     *time.Time       `gorm:"column:read_at;type:timestamptz"`
	ArchivedAt                 *time.Time       `gorm:"column:archived_at;type:timestamptz"`
	State                      LibraryItemState `gorm:"type:library_item_state;not null;default:'SUCCEEDED'"`
	ReadingProgressPercent     float64          `gorm:"column:reading_progress_top_percent;type:real;default:0"`
	ReadingProgressAnchorIndex int              `gorm:"column:reading_progress_highest_read_anchor;type:integer;default:0"`
	WordCount                  *int             `gorm:"column:word_count;type:integer"`
	Folder                     string           `gorm:"type:text;not null;default:'inbox'"`
	Subscription               *string          `gorm:"type:text"`
	ItemLanguage               *string          `gorm:"column:item_language;type:text"`
	ItemType                   *ContentReader   `gorm:"column:content_reader;type:content_reader_type"`
	TempPDFURL                 *string          `gorm:"-"`
	Site                       *string          `gorm:"column:site_name;type:text"`
	SiteIcon                   *string          `gorm:"column:site_icon;type:text"`
	UploadFileID               *uuid.UUID       `gorm:"column:upload_file_id;type:uuid"`
	DirectFileURL              *string          `gorm:"column:download_url;type:text"`
	CreatedAt                  time.Time        `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt                  time.Time        `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User       *User       `gorm:"foreignKey:UserID"`
	Highlights []Highlight `gorm:"foreignKey:LibraryItemID"`
	Labels     []Label     `gorm:"many2many:omnivore.entity_labels;foreignKey:ID;joinForeignKey:library_item_id;References:ID;joinReferences:label_id"`
}

func (LibraryItem) TableName() string {
	return "omnivore.library_item"
}
