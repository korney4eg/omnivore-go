package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

// HandleSyncReadPositions flushes buffered reading-progress updates from Redis into PostgreSQL.
func HandleSyncReadPositions(ctx context.Context, cfg *config.Config, redisDS *redisutil.RedisDataSource, dbPool *db.Pool, data []byte) error {
	client := redisDS.CacheClient

	var cursor uint64
	const pattern = "omnivore:reading-progress:*"
	const count = 100

	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, pattern, count).Result()
		if err != nil {
			return fmt.Errorf("sync-read-positions: scan: %w", err)
		}

		for _, key := range keys {
			if err := syncReadKey(ctx, client, dbPool, key); err != nil {
				log.Printf("[sync-read-positions] key=%s error: %v", key, err)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

type readingProgressEntry struct {
	UID                        string    `json:"uid"`
	LibraryItemID              string    `json:"libraryItemID"`
	ReadingProgressPercent     float64   `json:"readingProgressPercent"`
	ReadingProgressTopPercent  float64   `json:"readingProgressTopPercent"`
	ReadingProgressAnchorIndex int       `json:"readingProgressAnchorIndex"`
	UpdatedAt                  time.Time `json:"updatedAt"`
}

func syncReadKey(ctx context.Context, client *redis.Client, dbPool *db.Pool, key string) error {
	// Parse key: omnivore:reading-progress:<userId>:<libraryItemId>
	parts := strings.SplitN(key, ":", 4)
	if len(parts) < 4 {
		return fmt.Errorf("unexpected key format: %s", key)
	}
	userID := parts[2]
	libraryItemID := parts[3]

	members, err := client.SMembers(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("smembers: %w", err)
	}
	if len(members) == 0 {
		return nil
	}

	var entries []readingProgressEntry
	for _, m := range members {
		var e readingProgressEntry
		if err := json.Unmarshal([]byte(m), &e); err != nil {
			log.Printf("[sync-read-positions] skipping invalid member in key=%s: %v", key, err)
			continue
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return nil
	}

	// Find the most recent updatedAt across all entries.
	var mostRecent time.Time
	for _, e := range entries {
		if e.UpdatedAt.After(mostRecent) {
			mostRecent = e.UpdatedAt
		}
	}

	// Skip if still fresh (may still be updating).
	if time.Since(mostRecent) < 60*time.Second {
		return nil
	}

	// Aggregate max values.
	var topPercent, bottomPercent float64
	var anchorIndex int
	for _, e := range entries {
		if e.ReadingProgressTopPercent > topPercent {
			topPercent = e.ReadingProgressTopPercent
		}
		if e.ReadingProgressPercent > bottomPercent {
			bottomPercent = e.ReadingProgressPercent
		}
		if e.ReadingProgressAnchorIndex > anchorIndex {
			anchorIndex = e.ReadingProgressAnchorIndex
		}
	}

	if err := dbPool.AuthTrx(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
		UPDATE omnivore.library_item
		SET
		  reading_progress_top_percent = CASE
		    WHEN reading_progress_top_percent < $2 THEN $2
		    WHEN $2 = 0 THEN 0
		    ELSE reading_progress_top_percent END,
		  reading_progress_bottom_percent = CASE
		    WHEN reading_progress_bottom_percent < $3 THEN $3
		    WHEN $3 = 0 THEN 0
		    ELSE reading_progress_bottom_percent END,
		  reading_progress_highest_read_anchor = CASE
		    WHEN reading_progress_top_percent < $4 THEN $4
		    WHEN $4 = 0 THEN 0
		    ELSE reading_progress_highest_read_anchor END,
		  read_at = now()
		WHERE id = $1 AND user_id = $5 AND (
		  (reading_progress_top_percent < $2 OR $2 = 0) OR
		  (reading_progress_bottom_percent < $3 OR $3 = 0) OR
		  (reading_progress_highest_read_anchor < $4 OR $4 = 0)
		)
	`, libraryItemID, topPercent, bottomPercent, anchorIndex, userID)
		return err
	}); err != nil {
		return fmt.Errorf("update progress: %w", err)
	}

	// Remove processed members.
	raw := make([]interface{}, len(members))
	for i, m := range members {
		raw[i] = m
	}
	if err := client.SRem(ctx, key, raw...).Err(); err != nil {
		log.Printf("[sync-read-positions] srem key=%s error: %v", key, err)
	}
	return nil
}
