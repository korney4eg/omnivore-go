package model

import (
	"time"

	"github.com/google/uuid"
)

// Webhook represents a webhook configuration for user events.
// Table: omnivore.webhooks
type Webhook struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID      uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_webhooks_user_id"`
	URL         string     `gorm:"type:text;not null"`
	EventTypes  []string   `gorm:"column:event_types;type:text[];not null"` // Array type
	Enabled     bool       `gorm:"type:boolean;not null;default:true"`
	Method      string     `gorm:"type:text;not null;default:'POST'"`
	ContentType string     `gorm:"column:content_type;type:text;not null;default:'application/json'"`
	CreatedAt   time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt   time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (Webhook) TableName() string {
	return "omnivore.webhooks"
}

// ApiKey represents an API key for programmatic access.
// Table: omnivore.api_keys
type ApiKey struct {
	ID         uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID     uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_api_keys_user_id"`
	Name       string     `gorm:"type:text;not null"`
	Key        string     `gorm:"type:text;not null;uniqueIndex"` // SHA256 hash
	Scopes     []string   `gorm:"type:text[]"` // Array type
	UsedAt     *time.Time `gorm:"column:used_at;type:timestamptz"`
	ExpiresAt  *time.Time `gorm:"column:expires_at;type:timestamptz"`
	CreatedAt  time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (ApiKey) TableName() string {
	return "omnivore.api_keys"
}

// Integration represents a third-party service integration.
// Table: omnivore.integrations
type Integration struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID    uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_integrations_user_id"`
	Name      string     `gorm:"type:text;not null"`
	Type      string     `gorm:"type:text;not null"` // e.g., "READWISE", "POCKET"
	Token     string     `gorm:"type:text;not null"` // Encrypted token
	Enabled   bool       `gorm:"type:boolean;not null;default:true"`
	Settings  *string    `gorm:"type:jsonb"` // JSON settings
	SyncedAt  *time.Time `gorm:"column:synced_at;type:timestamptz"`
	TaskName  *string    `gorm:"column:task_name;type:text"`
	CreatedAt time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (Integration) TableName() string {
	return "omnivore.integrations"
}
