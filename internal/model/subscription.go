package model

import (
	"time"

	"github.com/google/uuid"
)

// SubscriptionStatus represents the status of a subscription.
type SubscriptionStatus string

const (
	SubscriptionStatusActive       SubscriptionStatus = "ACTIVE"
	SubscriptionStatusUnsubscribed SubscriptionStatus = "UNSUBSCRIBED"
	SubscriptionStatusDeleted      SubscriptionStatus = "DELETED"
)

// SubscriptionType represents the type of subscription.
type SubscriptionType string

const (
	SubscriptionTypeRSS        SubscriptionType = "RSS"
	SubscriptionTypeNewsletter SubscriptionType = "NEWSLETTER"
)

// Subscription represents a user's RSS feed or newsletter subscription.
// Table: omnivore.subscriptions
type Subscription struct {
	ID                  uuid.UUID          `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID              uuid.UUID          `gorm:"column:user_id;type:uuid;not null;index:idx_subscriptions_user_id"`
	Name                string             `gorm:"type:text;not null"`
	Description         *string            `gorm:"type:text"`
	URL                 *string            `gorm:"type:text"`
	FeedURL             *string            `gorm:"-"`
	Icon                *string            `gorm:"type:text"`
	Status              SubscriptionStatus `gorm:"type:subscription_status;not null;default:'ACTIVE'"`
	Type                SubscriptionType   `gorm:"type:subscription_type;not null;default:'RSS'"`
	LastFetchedAt       *time.Time         `gorm:"-"`
	LastFetchedChecksum *string            `gorm:"column:last_fetched_checksum;type:text"`
	FetchContentType    string             `gorm:"column:fetch_content_type;type:text;not null;default:'never'"`
	Count               int                `gorm:"type:integer;not null;default:0"`
	UnreadCount         int                `gorm:"-"`
	AutoAddToLibrary    bool               `gorm:"column:auto_add_to_library;type:boolean;not null;default:false"`
	FetchContent        bool               `gorm:"column:fetch_content;type:boolean;not null;default:false"`
	Folder              string             `gorm:"type:text;not null;default:'following'"`
	IsPrivate           *bool              `gorm:"column:is_private;type:boolean"`
	MostRecentItemDate  *time.Time         `gorm:"column:most_recent_item_date;type:timestamptz"`
	RefreshedAt         *time.Time         `gorm:"column:refreshed_at;type:timestamptz"`
	ScheduledAt         *time.Time         `gorm:"column:scheduled_at;type:timestamptz"`
	FailedAt            *time.Time         `gorm:"column:failed_at;type:timestamptz"`
	UnsubscribeMailTo   *string            `gorm:"column:unsubscribe_mail_to;type:text"`
	UnsubscribeHttpUrl  *string            `gorm:"column:unsubscribe_http_url;type:text"`
	NewsletterEmailID   *uuid.UUID         `gorm:"column:newsletter_email_id;type:uuid"`
	CreatedAt           time.Time          `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt           time.Time          `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User            *User            `gorm:"foreignKey:UserID"`
	NewsletterEmail *NewsletterEmail `gorm:"foreignKey:NewsletterEmailID"`
}

func (Subscription) TableName() string {
	return "omnivore.subscriptions"
}

// Feed represents an RSS/Atom feed.
// Table: omnivore.feeds
type Feed struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	Title         *string    `gorm:"type:text"`
	Description   *string    `gorm:"type:text"`
	URL           string     `gorm:"type:text;not null;uniqueIndex"`
	Type          string     `gorm:"type:text;not null;default:'RSS'"`
	Image         *string    `gorm:"type:text"`
	Icon          *string    `gorm:"type:text"`
	LastFetchedAt *time.Time `gorm:"column:last_fetched_at;type:timestamptz"`
	FetchedAt     *time.Time `gorm:"column:fetched_at;type:timestamptz"`
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
}

func (Feed) TableName() string {
	return "omnivore.feeds"
}

// NewsletterEmail represents an email address used for newsletter subscriptions.
// Table: omnivore.newsletter_emails
type NewsletterEmail struct {
	ID               uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID           uuid.UUID `gorm:"column:user_id;type:uuid;not null;index:idx_newsletter_emails_user_id"`
	Address          string    `gorm:"type:text;not null;uniqueIndex"`
	ConfirmationCode *string   `gorm:"column:confirmation_code;type:text"`
	Folder           string    `gorm:"type:text;not null;default:'following'"`
	Description      *string   `gorm:"type:text"`
	Name             *string   `gorm:"type:text"`
	CreatedAt        time.Time `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (NewsletterEmail) TableName() string {
	return "omnivore.newsletter_emails"
}

// ReceivedEmail represents an email received via newsletter subscription.
// Table: omnivore.received_emails
type ReceivedEmail struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID     uuid.UUID `gorm:"column:user_id;type:uuid;not null;index:idx_received_emails_user_id"`
	From       string    `gorm:"type:text;not null"`
	To         string    `gorm:"type:text;not null"`
	Subject    string    `gorm:"type:text;not null"`
	Text       *string   `gorm:"type:text"`
	HTML       *string   `gorm:"type:text"`
	Type       string    `gorm:"type:text;not null"`
	ReceivedAt time.Time `gorm:"column:received_at;type:timestamptz;not null;default:current_timestamp"`
	CreatedAt  time.Time `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (ReceivedEmail) TableName() string {
	return "omnivore.received_emails"
}
