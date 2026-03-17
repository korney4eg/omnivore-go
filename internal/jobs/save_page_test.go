package jobs_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"

	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/jobs"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

// mockDB records upsert calls without touching real Postgres.
type mockDB struct {
	execSQL  []string
	execArgs [][]any
}

func (m *mockDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	// Return ErrNoRows so HandleSavePage always inserts rather than updates.
	return &mockRow{err: pgx.ErrNoRows}
}

func (m *mockDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.execSQL = append(m.execSQL, sql)
	m.execArgs = append(m.execArgs, args)
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

// mockRow implements pgx.Row for QueryRow.
type mockRow struct{ err error }

func (r *mockRow) Scan(dest ...any) error { return r.err }

// TestHandleSavePage_FromCache verifies that HandleSavePage:
//  1. Reads HTML from the Redis cache.
//  2. Parses it with go-readability (title non-empty, word count > 0).
//  3. Calls Exec on the DB with state = "SUCCEEDED".
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

	mdb := &mockDB{}

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

	if err := jobs.HandleSavePage(ctx, cfg, redisDS, mdb, rawData); err != nil {
		t.Fatalf("HandleSavePage returned error: %v", err)
	}

	// Verify the mock DB received exactly one Exec call (the INSERT).
	if len(mdb.execSQL) != 1 {
		t.Fatalf("expected 1 DB Exec call, got %d", len(mdb.execSQL))
	}

	// The INSERT args: $1=id, $2=userId, $3=slug, $4=title, ...
	// Index 3 is title (0-based), index 8 is wordCount, index 9 is state.
	args := mdb.execArgs[0]
	if len(args) < 10 {
		t.Fatalf("expected ≥10 INSERT args, got %d", len(args))
	}

	title, ok := args[3].(string)
	if !ok || title == "" {
		t.Errorf("expected non-empty title arg, got %v", args[3])
	}

	wordCount, ok := args[8].(int)
	if !ok || wordCount <= 0 {
		t.Errorf("expected word count > 0, got %v", args[8])
	}

	state, ok := args[9].(string)
	if !ok || state != "SUCCEEDED" {
		t.Errorf("expected state=SUCCEEDED, got %v", args[9])
	}

	t.Logf("save-page: title=%q words=%d state=%s", title, wordCount, state)
}

