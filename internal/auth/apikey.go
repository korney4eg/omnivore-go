package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// APIKeyConfig holds API key configuration.
type APIKeyConfig struct {
	DB          *gorm.DB
	RedisClient *redis.Client
	CacheTTL    time.Duration
}

// APIKey represents an API key in the database.
type APIKey struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v1mc()"`
	UserID      uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_api_keys_user_id"`
	Name        string     `gorm:"type:text;not null"`
	KeyHash     string     `gorm:"column:key_hash;type:text;not null;uniqueIndex"`
	Scopes      []string   `gorm:"type:text[];column:scopes"`
	ExpiresAt   *time.Time `gorm:"column:expires_at;type:timestamptz"`
	LastUsedAt  *time.Time `gorm:"column:last_used_at;type:timestamptz"`
	CreatedAt   time.Time  `gorm:"type:timestamptz;not null;default:current_timestamp"`
	RevokedAt   *time.Time `gorm:"column:revoked_at;type:timestamptz"`
}

func (APIKey) TableName() string {
	return "omnivore.api_keys"
}

// APIKeyInfo holds validated API key information.
type APIKeyInfo struct {
	UserID    uuid.UUID
	UserRole  string
	Scopes    []string
	ExpiresAt *time.Time
}

// GenerateAPIKey creates a new API key.
func (c *APIKeyConfig) GenerateAPIKey(ctx context.Context, userID uuid.UUID, name string, scopes []string, expiresAt *time.Time) (string, error) {
	// Generate random key (32 bytes = 64 hex chars)
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}
	
	key := hex.EncodeToString(keyBytes)
	
	// Hash the key for storage (SHA256)
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	// Store in database
	apiKey := APIKey{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      name,
		KeyHash:   keyHash,
		Scopes:    scopes,
		ExpiresAt: expiresAt,
	}

	if err := c.DB.WithContext(ctx).Create(&apiKey).Error; err != nil {
		return "", fmt.Errorf("failed to store API key: %w", err)
	}

	// Return the plain key (only time it's visible)
	return key, nil
}

// ValidateAPIKey validates an API key and returns user info.
func (c *APIKeyConfig) ValidateAPIKey(ctx context.Context, key string) (*APIKeyInfo, error) {
	// Hash the key
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	// Check Redis cache first
	if c.RedisClient != nil {
		cacheKey := fmt.Sprintf("apikey:%s", keyHash)
		cached, err := c.RedisClient.Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			// Parse cached result
			var info APIKeyInfo
			// Simple cache format: userID|role|scopes
			// For production, use JSON or msgpack
			_, _ = fmt.Sscanf(cached, "%s", &info.UserID)
			
			// Update last used asynchronously
			go c.updateLastUsed(keyHash)
			
			return &info, nil
		}
	}

	// Query database
	var apiKey APIKey
	err := c.DB.WithContext(ctx).
		Preload("User").
		Where("key_hash = ?", keyHash).
		Where("revoked_at IS NULL").
		First(&apiKey).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("invalid API key")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query API key: %w", err)
	}

	// Check expiration
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("API key expired")
	}

	info := &APIKeyInfo{
		UserID:    apiKey.UserID,
		UserRole:  "omnivore_user", // Default role
		Scopes:    apiKey.Scopes,
		ExpiresAt: apiKey.ExpiresAt,
	}

	// Cache the result
	if c.RedisClient != nil {
		cacheKey := fmt.Sprintf("apikey:%s", keyHash)
		cacheValue := fmt.Sprintf("%s", info.UserID.String())
		c.RedisClient.Set(ctx, cacheKey, cacheValue, c.CacheTTL)
	}

	// Update last used
	go c.updateLastUsed(keyHash)

	return info, nil
}

// RevokeAPIKey revokes an API key.
func (c *APIKeyConfig) RevokeAPIKey(ctx context.Context, keyID uuid.UUID) error {
	now := time.Now()
	
	err := c.DB.WithContext(ctx).
		Model(&APIKey{}).
		Where("id = ?", keyID).
		Update("revoked_at", now).Error

	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	return nil
}

// ListAPIKeys lists all API keys for a user.
func (c *APIKeyConfig) ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]APIKey, error) {
	var keys []APIKey
	
	err := c.DB.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("revoked_at IS NULL").
		Order("created_at DESC").
		Find(&keys).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	return keys, nil
}

// updateLastUsed updates the last_used_at timestamp (async, best effort).
func (c *APIKeyConfig) updateLastUsed(keyHash string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	c.DB.WithContext(ctx).
		Model(&APIKey{}).
		Where("key_hash = ?", keyHash).
		Update("last_used_at", now)
}
