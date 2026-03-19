package service

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/bullmq"
	"github.com/omnivore-app/omnivore/internal/model"
	"github.com/omnivore-app/omnivore/internal/repository"
	"github.com/redis/go-redis/v9"
)

// SaveURLService handles URL saving and content fetching.
type SaveURLService struct {
	libraryItemRepo *repository.LibraryItemRepository
	redisClient     *redis.Client
}

// NewSaveURLService creates a new save URL service.
func NewSaveURLService(
	libraryItemRepo *repository.LibraryItemRepository,
	redisClient *redis.Client,
) *SaveURLService {
	return &SaveURLService{
		libraryItemRepo: libraryItemRepo,
		redisClient:     redisClient,
	}
}

// SaveURLInput holds input for saving a URL.
type SaveURLInput struct {
	UserID          uuid.UUID
	URL             string
	Source          string
	ClientRequestID string
	Timezone        *string
	Locale          *string
	Labels          []string
}

// SaveURLResult holds the result of saving a URL.
type SaveURLResult struct {
	URL             string
	ClientRequestID string
	LibraryItemID   uuid.UUID
}

// SaveURL validates a URL, creates/updates a library item, and enqueues content fetch.
func (s *SaveURLService) SaveURL(ctx context.Context, input SaveURLInput) (*SaveURLResult, error) {
	// Validate URL
	cleanedURL, err := validateAndCleanURL(input.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Check for existing library item
	existing, err := s.libraryItemRepo.FindByURL(ctx, input.UserID, cleanedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing item: %w", err)
	}

	var libraryItemID uuid.UUID

	if existing != nil {
		// Update existing item
		libraryItemID = existing.ID
		existing.State = model.LibraryItemStateProcessing
		existing.SavedAt = time.Now()

		if err := s.libraryItemRepo.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update existing item: %w", err)
		}
	} else {
		// Create new library item
		libraryItemID = uuid.New()
		slug := generateSlug(cleanedURL)
		contentReader := model.ContentReaderWeb
		readableContent := ""

		item := &model.LibraryItem{
			ID:              libraryItemID,
			UserID:          input.UserID,
			OriginalURL:     cleanedURL,
			Slug:            slug,
			Title:           &cleanedURL, // Default to URL
			State:           model.LibraryItemStateProcessing,
			ItemType:        &contentReader,
			ReadableContent: &readableContent,
			Folder:          "inbox",
			SavedAt:         time.Now(),
		}

		if err := s.libraryItemRepo.Create(ctx, item); err != nil {
			return nil, fmt.Errorf("failed to create library item: %w", err)
		}
	}

	// Enqueue content fetch job
	jobData := map[string]interface{}{
		"url": cleanedURL,
		"users": []map[string]interface{}{
			{
				"id":            input.UserID.String(),
				"libraryItemId": libraryItemID.String(),
				"folder":        "inbox",
			},
		},
		"priority": "high",
		"state":    "PROCESSING",
	}

	if input.Timezone != nil {
		jobData["timezone"] = *input.Timezone
	}
	if input.Locale != nil {
		jobData["locale"] = *input.Locale
	}
	if len(input.Labels) > 0 {
		jobData["labels"] = input.Labels
	}

	jobID := fmt.Sprintf("fetch-content_%s", input.ClientRequestID)

	job := bullmq.AddJobOpts{
		Name:  "fetch-content",
		Data:  jobData,
		JobID: jobID,
		Opts: bullmq.JobOpts{
			Attempts: 2,
			Priority: 1, // High priority
			Backoff: bullmq.BackoffOpt{
				Type:  "exponential",
				Delay: 2000,
			},
		},
	}

	if err := bullmq.AddBulk(ctx, s.redisClient, bullmq.ContentFetchQueue, []bullmq.AddJobOpts{job}); err != nil {
		return nil, fmt.Errorf("failed to enqueue fetch job: %w", err)
	}

	return &SaveURLResult{
		URL:             cleanedURL,
		ClientRequestID: input.ClientRequestID,
		LibraryItemID:   libraryItemID,
	}, nil
}

// validateAndCleanURL validates and cleans a URL.
func validateAndCleanURL(rawURL string) (string, error) {
	// Parse URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Protocol validation
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid protocol: %s (must be http or https)", u.Scheme)
	}

	// Localhost/loopback rejection
	if u.Hostname() == "localhost" || u.Hostname() == "0.0.0.0" || u.Hostname() == "127.0.0.1" {
		return "", fmt.Errorf("localhost URLs are not allowed")
	}

	// GCP metadata domains rejection
	if u.Hostname() == "metadata.google.internal" {
		return "", fmt.Errorf("GCP metadata URLs are not allowed")
	}

	// 169.254.x.x (link-local) rejection
	if strings.HasPrefix(u.Hostname(), "169.254.") {
		return "", fmt.Errorf("link-local URLs are not allowed")
	}

	// Private IP ranges rejection (simplified - 10.x.x.x, 172.16-31.x.x, 192.168.x.x)
	if isPrivateIP(u.Hostname()) {
		return "", fmt.Errorf("private IP addresses are not allowed")
	}

	// Clean tracking parameters
	cleaned := cleanTrackingParams(u)

	return cleaned, nil
}

// isPrivateIP checks if hostname is a private IP range.
func isPrivateIP(hostname string) bool {
	privateRanges := []string{
		`^10\.`,                         // 10.0.0.0/8
		`^172\.(1[6-9]|2[0-9]|3[01])\.`, // 172.16.0.0/12
		`^192\.168\.`,                   // 192.168.0.0/16
	}

	for _, pattern := range privateRanges {
		matched, _ := regexp.MatchString(pattern, hostname)
		if matched {
			return true
		}
	}
	return false
}

// cleanTrackingParams removes common tracking parameters from URL.
func cleanTrackingParams(u *url.URL) string {
	trackingParams := map[string]bool{
		"utm_source":   true,
		"utm_medium":   true,
		"utm_campaign": true,
		"utm_term":     true,
		"utm_content":  true,
		"fbclid":       true,
		"gclid":        true,
		"msclkid":      true,
	}

	query := u.Query()
	for param := range trackingParams {
		query.Del(param)
	}

	u.RawQuery = query.Encode()
	return u.String()
}

// generateSlug creates a URL-based slug.
func generateSlug(rawURL string) string {
	u, _ := url.Parse(rawURL)

	// Use hostname + path
	slug := u.Hostname() + u.Path

	// Clean up
	slug = strings.TrimSuffix(slug, "/")
	slug = strings.ReplaceAll(slug, "/", "-")
	slug = strings.ReplaceAll(slug, ".", "-")
	slug = strings.ToLower(slug)

	// Limit length
	if len(slug) > 100 {
		slug = slug[:100]
	}

	// Add random suffix for uniqueness
	slug = fmt.Sprintf("%s-%d", slug, time.Now().UnixNano()%1000000)

	return slug
}
