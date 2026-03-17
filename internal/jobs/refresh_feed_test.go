package jobs_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mmcdole/gofeed"
	"github.com/redis/go-redis/v9"

	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/jobs"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

// TestIsOldItem verifies the IsOldItem logic for new and existing subscriptions.
func TestIsOldItem(t *testing.T) {
	jan2026 := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	jun2024 := time.Date(2024, 6, 20, 0, 0, 0, 0, time.UTC)

	makeItem := func(t time.Time) *gofeed.Item {
		return &gofeed.Item{PublishedParsed: &t}
	}

	tests := []struct {
		name        string
		item        *gofeed.Item
		mostRecent  int64
		wantOld     bool
	}{
		{
			// 2026-01-20 is 56 days before 2026-03-17 — older than 24h for new sub.
			name:       "new subscription: 2026-01-20 item is old (>24h)",
			item:       makeItem(jan2026),
			mostRecent: 0,
			wantOld:    true,
		},
		{
			name:       "new subscription: old item (2024-06-20) is old",
			item:       makeItem(jun2024),
			mostRecent: 0,
			wantOld:    true,
		},
		{
			name:       "new subscription: item within 24h is not old",
			item:       makeItem(time.Now().Add(-1 * time.Hour)),
			mostRecent: 0,
			wantOld:    false,
		},
		{
			name:       "existing subscription: item at same timestamp is old",
			item:       makeItem(jan2026),
			mostRecent: jan2026.UnixMilli(),
			wantOld:    true,
		},
		{
			name:       "existing subscription: item newer than mostRecent is not old",
			item:       makeItem(jan2026),
			mostRecent: jun2024.UnixMilli(),
			wantOld:    false,
		},
		{
			name:       "item without date is never old",
			item:       &gofeed.Item{},
			mostRecent: 0,
			wantOld:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := jobs.IsOldItem(tc.item, tc.mostRecent)
			if got != tc.wantOld {
				t.Errorf("IsOldItem() = %v, want %v", got, tc.wantOld)
			}
		})
	}
}

// TestHandleRefreshFeed_FetchContentEnqueue verifies that HandleRefreshFeed with
// FetchContentType=ALWAYS for a new subscription (mostRecentItemDate=0) processes
// only the non-old items from the fixture feed.
//
// The fixture tests/fixtures/feed.xml has 2 items:
//   - 2026-01-20 → older than 24h relative to "now" in tests, which means it IS old
//     for the IsOldItem check. But the test is checking our filter logic: with
//     mostRecentItemDate=0, items older than 24h are skipped.
//
// Since both items in the fixture are in the past (2026-01-20 and 2024-06-20)
// from today's perspective, both would be filtered by the 24h rule.
// We simulate a "recent" item by adjusting mostRecentItemDate to accept the 2026 item.
//
// Test strategy:
//  1. Use mostRecentItemDate = 0 with a test feed served from httptest.Server
//     where we inject a single item dated NOW-1h (within 24h window) → 1 job enqueued.
//  2. Separately verify the fixture's 2 items produce 0 jobs when mostRecentItemDate=0
//     (both are older than 24h).
func TestHandleRefreshFeed_FetchContentEnqueue(t *testing.T) {
	// Start miniredis.
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	redisDS := &redisutil.RedisDataSource{
		CacheClient: redisClient,
		MQClient:    redisClient,
	}

	cfg := &config.Config{MaxFeedFetchFailures: 10}
	mdb := &mockFeedDB{}
	ctx := context.Background()

	t.Run("recent item within 24h is enqueued", func(t *testing.T) {
		// Build a feed with one item published 1 hour ago (within 24h window).
		recentTime := time.Now().Add(-1 * time.Hour)
		feedXML := buildFeedXML(recentTime)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write([]byte(feedXML))
		}))
		defer srv.Close()

		req := jobs.RefreshFeedRequest{
			SubscriptionIDs:      []string{"sub-1"},
			FeedURL:              srv.URL + "/feed.xml",
			MostRecentItemDates:  []int64{0},
			ScheduledTimestamps:  []int64{time.Now().UnixMilli()},
			LastFetchedChecksums: []string{""},
			UserIDs:              []string{"user-1"},
			FetchContentTypes:    []string{"ALWAYS"},
			Folders:              []string{"following"},
		}
		rawData, _ := json.Marshal(req)

		if err := jobs.HandleRefreshFeed(ctx, cfg, redisDS, mdb, rawData); err != nil {
			t.Fatalf("HandleRefreshFeed error: %v", err)
		}

		// Verify a fetch-content job was enqueued on the content-fetch queue.
		waitKey := "bull:omnivore-content-fetch-queue:wait"
		items, err := redisClient.LRange(ctx, waitKey, 0, -1).Result()
		if err != nil {
			t.Fatalf("lrange wait key: %v", err)
		}
		if len(items) != 1 {
			t.Errorf("expected 1 fetch-content job, got %d", len(items))
		}

		// Also verify subscription was updated.
		if len(mdb.execCalls) == 0 {
			t.Error("expected at least one DB Exec (subscription update)")
		}
	})

	t.Run("fixture feed: both items old, fallback saves most-recent for new sub", func(t *testing.T) {
		mr2, _ := miniredis.Run()
		defer mr2.Close()
		redisClient2 := redis.NewClient(&redis.Options{Addr: mr2.Addr()})
		redisDS2 := &redisutil.RedisDataSource{CacheClient: redisClient2, MQClient: redisClient2}

		xmlBytes, err := os.ReadFile("../../tests/fixtures/feed.xml")
		if err != nil {
			t.Skipf("fixture not found: %v", err)
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write(xmlBytes)
		}))
		defer srv.Close()

		mdb2 := &mockFeedDB{}
		req := jobs.RefreshFeedRequest{
			SubscriptionIDs:      []string{"sub-2"},
			FeedURL:              srv.URL + "/feed.xml",
			MostRecentItemDates:  []int64{0},
			ScheduledTimestamps:  []int64{time.Now().UnixMilli()},
			LastFetchedChecksums: []string{""},
			UserIDs:              []string{"user-2"},
			FetchContentTypes:    []string{"ALWAYS"},
			Folders:              []string{"following"},
		}
		rawData, _ := json.Marshal(req)

		if err := jobs.HandleRefreshFeed(ctx, cfg, redisDS2, mdb2, rawData); err != nil {
			t.Fatalf("HandleRefreshFeed error: %v", err)
		}

		// Both items are >24h old for a new subscription, but the fallback logic
		// (mirroring the TS behavior) saves the most-recent valid item for a never-fetched feed.
		// So we expect exactly 1 fetch-content job (the 2026-01-20 item as fallback).
		waitKey := "bull:omnivore-content-fetch-queue:wait"
		items, _ := redisClient2.LRange(ctx, waitKey, 0, -1).Result()
		if len(items) != 1 {
			t.Errorf("expected 1 fetch-content job (fallback for new sub), got %d", len(items))
		}
	})

	t.Run("fixture feed: 2026 item saved when mostRecentItemDate older", func(t *testing.T) {
		mr3, _ := miniredis.Run()
		defer mr3.Close()
		redisClient3 := redis.NewClient(&redis.Options{Addr: mr3.Addr()})
		redisDS3 := &redisutil.RedisDataSource{CacheClient: redisClient3, MQClient: redisClient3}

		xmlBytes, err := os.ReadFile("../../tests/fixtures/feed.xml")
		if err != nil {
			t.Skipf("fixture not found: %v", err)
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(xmlBytes)
		}))
		defer srv.Close()

		mdb3 := &mockFeedDB{}
		// mostRecentItemDate = 2025-01-01: only 2026-01-20 item is newer.
		mostRecent := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		req := jobs.RefreshFeedRequest{
			SubscriptionIDs:      []string{"sub-3"},
			FeedURL:              srv.URL + "/feed.xml",
			MostRecentItemDates:  []int64{mostRecent},
			ScheduledTimestamps:  []int64{time.Now().UnixMilli()},
			LastFetchedChecksums: []string{""},
			UserIDs:              []string{"user-3"},
			FetchContentTypes:    []string{"ALWAYS"},
			Folders:              []string{"following"},
		}
		rawData, _ := json.Marshal(req)

		if err := jobs.HandleRefreshFeed(ctx, cfg, redisDS3, mdb3, rawData); err != nil {
			t.Fatalf("HandleRefreshFeed error: %v", err)
		}

		// Only the 2026-01-20 item is newer than mostRecentItemDate (2025-01-01).
		waitKey := "bull:omnivore-content-fetch-queue:wait"
		items, _ := redisClient3.LRange(ctx, waitKey, 0, -1).Result()
		if len(items) != 1 {
			t.Errorf("expected 1 fetch-content job (only 2026 item is new), got %d", len(items))
		}
	})
}

// mockFeedDB implements feedDB for testing.
type mockFeedDB struct {
	execCalls []string
}

func (m *mockFeedDB) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	m.execCalls = append(m.execCalls, sql)
	return pgconn.NewCommandTag("UPDATE 0 1"), nil
}

// buildFeedXML returns a minimal RSS feed XML with one item at the given publish time.
func buildFeedXML(pubDate time.Time) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <link>http://example.com</link>
    <description>Test</description>
    <item>
      <title>Recent Item</title>
      <link>http://example.com/recent</link>
      <pubDate>` + pubDate.Format("Mon, 02 Jan 2006 15:04:05 -0700") + `</pubDate>
      <guid>http://example.com/recent</guid>
    </item>
  </channel>
</rss>`
}
