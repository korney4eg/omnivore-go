package model

import (
	"time"

	"github.com/google/uuid"
)

// FeatureType represents different feature types.
type FeatureType string

const (
	FeatureTypeDefault     FeatureType = "DEFAULT"
	FeatureTypeOptIn       FeatureType = "OPT_IN"
	FeatureTypeOptOut      FeatureType = "OPT_OUT"
)

// Feature represents a feature flag.
// Table: omnivore.features
type Feature struct {
	ID        uuid.UUID   `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	Name      string      `gorm:"type:text;not null;uniqueIndex"`
	Token     string      `gorm:"type:text;not null"`
	GrantedAt *time.Time  `gorm:"column:granted_at;type:timestamptz"`
	ExpiresAt *time.Time  `gorm:"column:expires_at;type:timestamptz"`
	CreatedAt time.Time   `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt time.Time   `gorm:"type:timestamptz;not null;default:current_timestamp"`
}

func (Feature) TableName() string {
	return "omnivore.features"
}

// UserDeviceToken represents a push notification device token.
// Table: omnivore.user_device_tokens
type UserDeviceToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID    uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_user_device_tokens_user_id"`
	Token     string     `gorm:"type:text;not null;uniqueIndex"`
	CreatedAt time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (UserDeviceToken) TableName() string {
	return "omnivore.user_device_tokens"
}

// ServiceUsage tracks API usage and quotas.
// Table: omnivore.service_usage
type ServiceUsage struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID    uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_service_usage_user_id"`
	ServiceType string   `gorm:"column:service_type;type:text;not null"`
	UsageCount int       `gorm:"column:usage_count;type:integer;not null;default:0"`
	Period    string     `gorm:"type:text;not null"` // e.g., "2024-01"
	CreatedAt time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (ServiceUsage) TableName() string {
	return "omnivore.service_usage"
}

// FolderPolicy represents folder-level policies and settings.
// Table: omnivore.folder_policies
type FolderPolicy struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID        uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_folder_policies_user_id"`
	Folder        string     `gorm:"type:text;not null"`
	Action        string     `gorm:"type:text;not null"` // e.g., "ARCHIVE", "DELETE"
	AfterDays     *int       `gorm:"column:after_days;type:integer"`
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (FolderPolicy) TableName() string {
	return "omnivore.folder_policies"
}
