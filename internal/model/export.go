package model

import (
	"time"

	"github.com/google/uuid"
)

// ExportFormat represents the export file format.
type ExportFormat string

const (
	ExportFormatHTML     ExportFormat = "HTML"
	ExportFormatMarkdown ExportFormat = "MARKDOWN"
	ExportFormatPDF      ExportFormat = "PDF"
	ExportFormatJSON     ExportFormat = "JSON"
	ExportFormatCSV      ExportFormat = "CSV"
)

// ExportState represents the state of an export job.
type ExportState string

const (
	ExportStatePending    ExportState = "PENDING"
	ExportStateProcessing ExportState = "PROCESSING"
	ExportStateSucceeded  ExportState = "SUCCEEDED"
	ExportStateFailed     ExportState = "FAILED"
)

// Export represents a data export job.
// Table: omnivore.exports
type Export struct {
	ID          uuid.UUID    `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID      uuid.UUID    `gorm:"column:user_id;type:uuid;not null;index:idx_exports_user_id"`
	Format      ExportFormat `gorm:"type:export_format;not null"`
	State       ExportState  `gorm:"type:export_state;not null;default:'PENDING'"`
	TaskID      *string      `gorm:"column:task_id;type:text"`
	TotalItems  int          `gorm:"column:total_items;type:integer;not null;default:0"`
	ProcessedItems int       `gorm:"column:processed_items;type:integer;not null;default:0"`
	FileURL     *string      `gorm:"column:file_url;type:text"`
	ExpiresAt   *time.Time   `gorm:"column:expires_at;type:timestamptz"`
	CreatedAt   time.Time    `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt   time.Time    `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (Export) TableName() string {
	return "omnivore.exports"
}

// UploadFileStatus represents the upload file status.
type UploadFileStatus string

const (
	UploadFileStatusInitialized UploadFileStatus = "INITIALIZED"
	UploadFileStatusCompleted   UploadFileStatus = "COMPLETED"
)

// UploadFile represents an uploaded file (PDF, EPUB, etc.).
// Table: omnivore.upload_files
type UploadFile struct {
	ID            uuid.UUID        `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID        uuid.UUID        `gorm:"column:user_id;type:uuid;not null;index:idx_upload_files_user_id"`
	URL           string           `gorm:"type:text;not null"`
	FileName      string           `gorm:"column:file_name;type:text;not null"`
	ContentType   string           `gorm:"column:content_type;type:text;not null"`
	Status        UploadFileStatus `gorm:"type:upload_file_status;not null;default:'INITIALIZED'"`
	CreatedAt     time.Time        `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (UploadFile) TableName() string {
	return "omnivore.upload_files"
}
