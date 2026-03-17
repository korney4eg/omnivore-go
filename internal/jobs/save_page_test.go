package jobs_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/jobs"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

// TestHandleSavePage_FromCache verifies that HandleSavePageWithUpsert:
//  1. Reads HTML from the Redis cache.
//  2. Parses it with go-readability (title non-empty, word count > 0).
//  3. Calls the upsert function with state = "SUCCEEDED".
func TestHandleSavePage_FromCache(t *testing.T) {
	// Start an in-process Redis server.
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	// Load HTML fixture.
	htmlBytes, err := os.ReadFile("../../tests/fixtures/article.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// Store cache entry in miniredis.
	cacheKey := "test-cache-key-001"
	payload := map[string]string{
		"finalUrl": "http://example.com/article",
		"title":    "Test Article",
		"content":  string(htmlBytes),
	}
	payloadBytes, _ := json.Marshal(payload)
	mr.Set(cacheKey, string(payloadBytes))

	// Build a minimal config (no blob storage; cache path will be used).
	cfg := &config.Config{
		BlobStorageURL:  "",
		GCSUploadBucket: "",
	}

	// Build a fake RedisDataSource pointing at miniredis.
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	redisDS := &redisutil.RedisDataSource{
		CacheClient: redisClient,
		MQClient:    redisClient,
	}

	ctx := context.Background()
	savedAt := time.Now().UTC().Format(time.RFC3339)
	data := jobs.SavePageData{
		UserID:                 "user-123",
		URL:                    "http://example.com/article",
		FinalURL:               "http://example.com/article",
		ArticleSavingRequestID: "item-456",
		Source:                 "web",
		CacheKey:               cacheKey,
		SavedAt:                &savedAt,
	}
	rawData, _ := json.Marshal(data)

	var capturedItem jobs.LibraryItem
	var upsertCalled bool
	mockUpsert := func(ctx context.Context, item jobs.LibraryItem) error {
		capturedItem = item
		upsertCalled = true
		return nil
	}

	if err := jobs.HandleSavePageWithUpsert(ctx, cfg, redisDS, mockUpsert, rawData); err != nil {
		t.Fatalf("HandleSavePageWithUpsert returned error: %v", err)
	}

	if !upsertCalled {
		t.Fatal("expected upsert to be called")
	}
	if capturedItem.Title == "" {
		t.Errorf("expected non-empty title, got empty")
	}
	if capturedItem.WordCount <= 0 {
		t.Errorf("expected word count > 0, got %d", capturedItem.WordCount)
	}
	if capturedItem.State != "SUCCEEDED" {
		t.Errorf("expected state=SUCCEEDED, got %q", capturedItem.State)
	}
	t.Logf("save-page: title=%q words=%d state=%s", capturedItem.Title, capturedItem.WordCount, capturedItem.State)
}

