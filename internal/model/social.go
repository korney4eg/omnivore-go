package model

import (
	"time"

	"github.com/google/uuid"
)

// Group represents a social group for sharing content.
// Table: omnivore.groups
type Group struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	Name          string     `gorm:"type:text;not null"`
	Description   *string    `gorm:"type:text"`
	CreatedBy     uuid.UUID  `gorm:"column:created_by;type:uuid;not null"`
	OnlyAdminCanPost bool    `gorm:"column:only_admin_can_post;type:boolean;not null;default:false"`
	OnlyAdminCanSeeMembers bool `gorm:"column:only_admin_can_see_members;type:boolean;not null;default:false"`
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	Creator *User `gorm:"foreignKey:CreatedBy"`
}

func (Group) TableName() string {
	return "omnivore.groups"
}

// GroupMembership represents a user's membership in a group.
// Table: omnivore.group_membership
type GroupMembership struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID    uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_group_membership_user_id"`
	GroupID   uuid.UUID  `gorm:"column:group_id;type:uuid;not null;index:idx_group_membership_group_id"`
	Role      string     `gorm:"type:text;not null;default:'MEMBER'"` // ADMIN, MEMBER
	InvitedBy *uuid.UUID `gorm:"column:invited_by;type:uuid"`
	CreatedAt time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User    *User  `gorm:"foreignKey:UserID"`
	Group   *Group `gorm:"foreignKey:GroupID"`
	Inviter *User  `gorm:"foreignKey:InvitedBy"`
}

func (GroupMembership) TableName() string {
	return "omnivore.group_membership"
}

// Follower represents a follower relationship between users.
// Table: omnivore.followers
type Follower struct {
	ID         uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID     uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_followers_user_id"`
	FollowerID uuid.UUID  `gorm:"column:follower_id;type:uuid;not null;index:idx_followers_follower_id"`
	CreatedAt  time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User     *User `gorm:"foreignKey:UserID"`
	Follower *User `gorm:"foreignKey:FollowerID"`
}

func (Follower) TableName() string {
	return "omnivore.followers"
}

// Post represents a shared post in a group or publicly.
// Table: omnivore.posts
type Post struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID        uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_posts_user_id"`
	LibraryItemID uuid.UUID  `gorm:"column:library_item_id;type:uuid;not null;index:idx_posts_library_item"`
	GroupID       *uuid.UUID `gorm:"column:group_id;type:uuid;index:idx_posts_group_id"`
	Note          *string    `gorm:"type:text"`
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	UpdatedAt     time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`

	// Relationships
	User        *User        `gorm:"foreignKey:UserID"`
	LibraryItem *LibraryItem `gorm:"foreignKey:LibraryItemID"`
	Group       *Group       `gorm:"foreignKey:GroupID"`
}

func (Post) TableName() string {
	return "omnivore.posts"
}
