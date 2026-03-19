package resolver

import (
	"github.com/omnivore-app/omnivore/internal/repository"
	"github.com/omnivore-app/omnivore/internal/service"
	"github.com/omnivore-app/omnivore/internal/storage"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

type Resolver struct {
	DB               *gorm.DB
	RedisClient      *redis.Client
	UserRepo         *repository.UserRepository
	LibraryItemRepo  *repository.LibraryItemRepository
	LabelRepo        *repository.LabelRepository
	HighlightRepo    *repository.HighlightRepository
	SubscriptionRepo *repository.SubscriptionRepository
	SearchService    *service.SearchService
	SaveURLService   *service.SaveURLService
	SavePageService  *service.SavePageService
}

// NewResolver creates a new resolver with injected dependencies.
func NewResolver(db *gorm.DB, redisClient *redis.Client, storageClient *storage.Client) *Resolver {
	libraryItemRepo := repository.NewLibraryItemRepository(db)

	return &Resolver{
		DB:               db,
		RedisClient:      redisClient,
		UserRepo:         repository.NewUserRepository(db),
		LibraryItemRepo:  libraryItemRepo,
		LabelRepo:        repository.NewLabelRepository(db),
		HighlightRepo:    repository.NewHighlightRepository(db),
		SubscriptionRepo: repository.NewSubscriptionRepository(db),
		SearchService:    service.NewSearchService(db),
		SaveURLService:   service.NewSaveURLService(libraryItemRepo, redisClient),
		SavePageService:  service.NewSavePageService(libraryItemRepo, redisClient, storageClient),
	}
}
