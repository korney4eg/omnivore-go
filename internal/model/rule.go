package model

import (
	"time"

	"github.com/google/uuid"
)

// RuleAction represents the action type for a rule.
type RuleAction string

const (
	RuleActionAddLabel    RuleAction = "ADD_LABEL"
	RuleActionArchive     RuleAction = "ARCHIVE"
	RuleActionDelete      RuleAction = "DELETE"
	RuleActionMarkRead    RuleAction = "MARK_READ"
	RuleActionSendNotification RuleAction = "SEND_NOTIFICATION"
)

// RuleEventType represents when a rule should be triggered.
type RuleEventType string

const (
	RuleEventPageCreated  RuleEventType = "PAGE_CREATED"
	RuleEventPageUpdated  RuleEventType = "PAGE_UPDATED"
)

// Rule represents an automation rule.
// Table: omnivore.rules
type Rule struct {
	ID          uuid.UUID     `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID      uuid.UUID     `gorm:"column:user_id;type:uuid;not null;index:idx_rules_user_id"`
	Name        string        `gorm:"type:text;not null"`
	Description *string       `gorm:"type:text"`
	Filter      string        `gorm:"type:text;not null"` // Search query filter
	Actions     []RuleAction  `gorm:"type:rule_action[];not null"` // Array of actions
	EventTypes  []RuleEventType `gorm:"column:event_types;type:rule_event_type[];not null"` // When to trigger
	Enabled     bool          `gorm:"type:boolean;not null;default:true"`
	Position    int           `gorm:"type:integer;not null;default:0"`
	CreatedAt   time.Time     `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt   time.Time     `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (Rule) TableName() string {
	return "omnivore.rules"
}

// Filter represents a saved search filter.
// Table: omnivore.filters
type Filter struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID      uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_filters_user_id"`
	Name        string     `gorm:"type:text;not null"`
	Description *string    `gorm:"type:text"`
	Filter      string     `gorm:"type:text;not null"` // Search query
	Position    int        `gorm:"type:integer;not null;default:0"`
	Category    string     `gorm:"type:text;not null;default:'Search'"`
	Folder      string     `gorm:"type:text;not null;default:'inbox'"`
	Icon        *string    `gorm:"type:text"`
	Visible     bool       `gorm:"type:boolean;not null;default:true"`
	CreatedAt   time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt   time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (Filter) TableName() string {
	return "omnivore.filters"
}

// SearchHistory represents a user's search history.
// Table: omnivore.search_history
type SearchHistory struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID    uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_search_history_user_id"`
	Term      string     `gorm:"type:text;not null"`
	CreatedAt time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User *User `gorm:"foreignKey:UserID"`
}

func (SearchHistory) TableName() string {
	return "omnivore.search_history"
}
