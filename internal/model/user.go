// Package model contains GORM models for the Omnivore database.
// These models match the existing PostgreSQL schema defined in packages/db/migrations/.
// DO NOT use GORM auto-migration - schema is managed via SQL migrations.
package model

import (
	"time"

	"github.com/google/uuid"
)

// RegistrationType represents how a user registered.
type RegistrationType string

const (
	RegistrationGoogle RegistrationType = "GOOGLE"
	RegistrationApple  RegistrationType = "APPLE"
	RegistrationEmail  RegistrationType = "EMAIL"
)

// UserStatus represents the user account status.
type UserStatus string

const (
	UserStatusActive   UserStatus = "ACTIVE"
	UserStatusPending  UserStatus = "PENDING"
	UserStatusDeleted  UserStatus = "DELETED"
	UserStatusArchived UserStatus = "ARCHIVED"
)

// User represents a user account.
// Table: omnivore.user
type User struct {
	ID            uuid.UUID        `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	Name          string           `gorm:"type:text;not null"`
	Source        RegistrationType `gorm:"type:registration_type;not null"`
	Email         string           `gorm:"type:text"`
	SourceUserID  string           `gorm:"column:source_user_id;type:text;not null"`
	Password      *string          `gorm:"type:varchar(255)"`
	Status        UserStatus       `gorm:"type:user_status_type;not null;default:'ACTIVE'"`
	Phone         *string          `gorm:"type:text"`
	Picture       *string          `gorm:"type:text"`
	Bio           *string          `gorm:"type:text"`
	CreatedAt     time.Time        `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt     time.Time        `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	Profile              *Profile               `gorm:"foreignKey:UserID"`
	LibraryItems         []LibraryItem          `gorm:"foreignKey:UserID"`
	Labels               []Label                `gorm:"foreignKey:UserID"`
	Subscriptions        []Subscription         `gorm:"foreignKey:UserID"`
	UserPersonalization  *UserPersonalization   `gorm:"foreignKey:UserID"`
}

func (User) TableName() string {
	return "omnivore.user"
}

// Profile represents a user's public profile.
// Table: omnivore.user_profile
type Profile struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex"`
	Username  string     `gorm:"type:text;not null;uniqueIndex"`
	Bio       *string    `gorm:"type:text"`
	PictureURL *string   `gorm:"column:picture_url;type:text"`
	Private   bool       `gorm:"not null;default:false"`
	CreatedAt time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	User *User `gorm:"foreignKey:UserID"`
}

func (Profile) TableName() string {
	return "omnivore.user_profile"
}

// UserPersonalization stores user preferences and settings.
// Table: omnivore.user_personalization
type UserPersonalization struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID         uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex"`
	Theme          *string    `gorm:"type:text"`
	FontSize       *int       `gorm:"type:integer"`
	FontFamily     *string    `gorm:"type:text"`
	Margin         *int       `gorm:"type:integer"`
	LibraryLayoutType *string `gorm:"column:library_layout_type;type:text"`
	LabelsView     *string    `gorm:"column:labels_view;type:text"`
	Fields         *string    `gorm:"type:json"` // JSONB stored as string
	DigestConfig   *string    `gorm:"column:digest_config;type:jsonb"` // JSONB stored as string
	Shortcuts      *string    `gorm:"type:jsonb"` // JSONB stored as string
	CreatedAt      time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt      time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	User *User `gorm:"foreignKey:UserID"`
}

func (UserPersonalization) TableName() string {
	return "omnivore.user_personalization"
}
