package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/omnivore-app/omnivore/internal/bullmq"
	"github.com/omnivore-app/omnivore/internal/model"
	"github.com/omnivore-app/omnivore/internal/repository"
	"github.com/omnivore-app/omnivore/internal/storage"
	"github.com/redis/go-redis/v9"
)

// SavePageService handles saving pages with HTML content.
type SavePageService struct {
	libraryItemRepo *repository.LibraryItemRepository
	redisClient     *redis.Client
	storageClient   *storage.Client
}

// NewSavePageService creates a new save page service.
func NewSavePageService(
	libraryItemRepo *repository.LibraryItemRepository,
	redisClient *redis.Client,
	storageClient *storage.Client,
) *SavePageService {
	return &SavePageService{
		libraryItemRepo: libraryItemRepo,
		redisClient:     redisClient,
		storageClient:   storageClient,
	}
}

// SavePageInput holds input for saving a page.
type SavePageInput struct {
	UserID          uuid.UUID
	URL             string
	OriginalHTML    string
	Title           *string
	Source          string
	ClientRequestID string
}

// SavePageResult holds the result of saving a page.
type SavePageResult struct {
	URL             string
	ClientRequestID string
	LibraryItemID   uuid.UUID
}

// SavePage validates a page, uploads HTML to storage, creates library item, and enqueues parsing job.
func (s *SavePageService) SavePage(ctx context.Context, input SavePageInput) (*SavePageResult, error) {
	clientRequestID := input.ClientRequestID
	if clientRequestID == "" {
		clientRequestID = uuid.New().String()
	}

	// Validate URL (use same validation as SaveURL)
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

		if input.Title != nil {
			existing.Title = input.Title
		}

		if err := s.libraryItemRepo.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update existing item: %w", err)
		}
	} else {
		// Create new library item
		libraryItemID = uuid.New()
		slug := generateSlug(cleanedURL)
		contentReader := model.ContentReaderWeb
		readableContent := ""

		title := cleanedURL
		if input.Title != nil {
			title = *input.Title
		}

		item := &model.LibraryItem{
			ID:              libraryItemID,
			UserID:          input.UserID,
			OriginalURL:     cleanedURL,
			Slug:            slug,
			Title:           &title,
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

	// Upload original HTML to storage
	savedTimestamp := time.Now().Unix()
	users := []storage.UserRef{
		{
			ID:            input.UserID.String(),
			LibraryItemID: libraryItemID.String(),
		},
	}

	if err := s.storageClient.UploadOriginalContent(ctx, users, input.OriginalHTML, savedTimestamp); err != nil {
		return nil, fmt.Errorf("failed to upload HTML to storage: %w", err)
	}

	// Enqueue save-page job for parsing
	jobData := map[string]interface{}{
		"userId":                 input.UserID.String(),
		"url":                    cleanedURL,
		"articleSavingRequestId": libraryItemID.String(),
		"state":                  "PROCESSING",
		"labels":                 []string{},
		"source":                 input.Source,
		"folder":                 "inbox",
		"savedTimestamp":         savedTimestamp,
	}

	if input.Title != nil {
		jobData["title"] = *input.Title
	}

	// Generate deterministic job ID
	jobID := fmt.Sprintf("save-page_%s", libraryItemID.String())

	job := bullmq.AddJobOpts{
		Name:  bullmq.SavePageJob,
		Data:  jobData,
		JobID: jobID,
		Opts: bullmq.JobOpts{
			Attempts: 3,
			Priority: 1, // High priority
			Backoff: bullmq.BackoffOpt{
				Type:  "exponential",
				Delay: 2000,
			},
		},
	}

	if err := bullmq.AddBulk(ctx, s.redisClient, bullmq.BackendQueue, []bullmq.AddJobOpts{job}); err != nil {
		return nil, fmt.Errorf("failed to enqueue save-page job: %w", err)
	}

	return &SavePageResult{
		URL:             cleanedURL,
		ClientRequestID: clientRequestID,
		LibraryItemID:   libraryItemID,
	}, nil
}
