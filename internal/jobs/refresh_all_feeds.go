package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/omnivore-app/omnivore/internal/bullmq"
	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

// allFeedsDB is the minimal DB interface needed by HandleRefreshAllFeeds.
type allFeedsDB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// subscriptionGroup mirrors the SQL GROUP BY result used by refreshAllFeeds.
type subscriptionGroup struct {
	URL                string
	SubscriptionIDs    []string
	UserIDs            []string
	MostRecentItemDates []int64 // unix ms, 0 for NULL
	ScheduledDates     []int64 // unix ms
	Checksums          []string
	Folders            []string
	FetchContentTypes  []string
}

// refreshFeedPayload is the JSON payload for a refresh-feed job.
type refreshFeedPayload struct {
	SubscriptionIDs      []string        `json:"subscriptionIds"`
	FeedURL              string          `json:"feedUrl"`
	MostRecentItemDates  []int64         `json:"mostRecentItemDates"`
	ScheduledTimestamps  []int64         `json:"scheduledTimestamps"`
	LastFetchedChecksums []string        `json:"lastFetchedChecksums"`
	UserIDs              []string        `json:"userIds"`
	FetchContentTypes    []string        `json:"fetchContentTypes"`
	Folders              []string        `json:"folders"`
	RefreshContext       refreshContext  `json:"refreshContext"`
}

type refreshContext struct {
	Type      string `json:"type"`
	RefreshID string `json:"refreshID"`
	StartedAt string `json:"startedAt"`
}

// HandleRefreshAllFeeds handles the refresh-all-feeds job (empty payload).
// It queries all active RSS subscriptions and enqueues individual refresh-feed jobs.
func HandleRefreshAllFeeds(
	ctx context.Context,
	cfg *config.Config,
	redisDS *redisutil.RedisDataSource,
	dbq allFeedsDB,
	rawData json.RawMessage,
) error {
	rc := refreshContext{
		Type:      "all",
		RefreshID: uuid.New().String(),
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}

	groups, err := querySubscriptionGroups(ctx, dbq)
	if err != nil {
		return fmt.Errorf("query subscription groups: %w", err)
	}

	log.Printf("[refresh-all-feeds] refreshID=%s found %d subscription groups", rc.RefreshID, len(groups))

	var jobOpts []bullmq.AddJobOpts
	for _, g := range groups {
		if g.URL == "" || len(g.UserIDs) == 0 {
			log.Printf("[refresh-all-feeds] skipping group with empty url or no users")
			continue
		}

		payload := refreshFeedPayload{
			SubscriptionIDs:      g.SubscriptionIDs,
			FeedURL:              g.URL,
			MostRecentItemDates:  g.MostRecentItemDates,
			ScheduledTimestamps:  g.ScheduledDates,
			LastFetchedChecksums: g.Checksums,
			UserIDs:              g.UserIDs,
			FetchContentTypes:    g.FetchContentTypes,
			Folders:              g.Folders,
			RefreshContext:       rc,
		}

		jobOpts = append(jobOpts, bullmq.AddJobOpts{
			Name:  "refresh-feed",
			Data:  payload,
			JobID: refreshFeedJobID(g.URL, g.UserIDs),
			Opts: bullmq.JobOpts{
				Attempts: 3,
				Backoff:  bullmq.BackoffOpt{Type: "exponential", Delay: 1000},
			},
		})
	}

	if len(jobOpts) == 0 {
		log.Printf("[refresh-all-feeds] no jobs to enqueue")
		return nil
	}

	if err := bullmq.AddBulk(ctx, redisDS.MQClient, bullmq.BackendQueue, jobOpts); err != nil {
		return fmt.Errorf("enqueue refresh-feed jobs: %w", err)
	}

	log.Printf("[refresh-all-feeds] enqueued %d refresh-feed jobs", len(jobOpts))
	return nil
}

// refreshFeedJobID creates a deterministic job ID from the feed URL and sorted user list.
// Mirrors the TS logic: refresh-feed_<hash(url)>_<hash(sortedUserIds)>
func refreshFeedJobID(feedURL string, userIDs []string) string {
	sorted := make([]string, len(userIDs))
	copy(sorted, userIDs)
	sort.Strings(sorted)
	userList, _ := json.Marshal(sorted)

	urlHash := sha256hex16(feedURL)
	userHash := sha256hex16(string(userList))
	return fmt.Sprintf("refresh-feed_%s_%s", urlHash, userHash)
}

func sha256hex16(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8]) // 8 bytes = 16 hex chars
}

// querySubscriptionGroups fetches all active RSS subscriptions grouped by URL.
// Mirrors the SQL in refreshAllFeeds.ts.
func querySubscriptionGroups(ctx context.Context, dbq allFeedsDB) ([]subscriptionGroup, error) {
	// Note: the users table is omnivore.user (not omnivore.users).
	// setup_db.bash uses INSERT INTO omnivore.user confirming the table name.
	rows, err := dbq.Query(ctx, `
		SELECT
			url,
			ARRAY_AGG(s.id::text)                                       AS subscription_ids,
			ARRAY_AGG(s.user_id::text)                                  AS user_ids,
			ARRAY_AGG(s.most_recent_item_date)                          AS most_recent_item_dates,
			ARRAY_AGG(COALESCE(s.scheduled_at, NOW()))                  AS scheduled_dates,
			ARRAY_AGG(s.last_fetched_checksum)                          AS checksums,
			ARRAY_AGG(COALESCE(s.folder, 'following'))                  AS folders,
			ARRAY_AGG(s.fetch_content_type)                             AS fetch_content_types
		FROM omnivore.subscriptions s
		INNER JOIN omnivore.user u ON u.id = s.user_id AND u.status = 'ACTIVE'
		WHERE
			s.type = 'RSS'
			AND s.status = 'ACTIVE'
			AND (s.scheduled_at <= NOW() OR s.scheduled_at IS NULL)
		GROUP BY url
	`)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var groups []subscriptionGroup
	for rows.Next() {
		var (
			url             string
			subscriptionIDs pgtype.Array[pgtype.Text]
			userIDs         pgtype.Array[pgtype.Text]
			recentDates     pgtype.Array[pgtype.Timestamptz]
			scheduledDates  pgtype.Array[pgtype.Timestamptz]
			checksums       pgtype.Array[pgtype.Text]
			folders         pgtype.Array[pgtype.Text]
			fetchTypes      pgtype.Array[pgtype.Text]
		)

		if err := rows.Scan(&url,
			&subscriptionIDs, &userIDs,
			&recentDates, &scheduledDates,
			&checksums, &folders, &fetchTypes,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		g := subscriptionGroup{URL: url}
		for _, e := range subscriptionIDs.Elements {
			if e.Valid {
				g.SubscriptionIDs = append(g.SubscriptionIDs, e.String)
			}
		}
		for _, e := range userIDs.Elements {
			if e.Valid {
				g.UserIDs = append(g.UserIDs, e.String)
			}
		}
		for _, e := range recentDates.Elements {
			if e.Valid {
				g.MostRecentItemDates = append(g.MostRecentItemDates, e.Time.UnixMilli())
			} else {
				g.MostRecentItemDates = append(g.MostRecentItemDates, 0)
			}
		}
		for _, e := range scheduledDates.Elements {
			if e.Valid {
				g.ScheduledDates = append(g.ScheduledDates, e.Time.UnixMilli())
			} else {
				g.ScheduledDates = append(g.ScheduledDates, time.Now().UnixMilli())
			}
		}
		for _, e := range checksums.Elements {
			if e.Valid {
				g.Checksums = append(g.Checksums, e.String)
			} else {
				g.Checksums = append(g.Checksums, "")
			}
		}
		for _, e := range folders.Elements {
			if e.Valid {
				g.Folders = append(g.Folders, e.String)
			} else {
				g.Folders = append(g.Folders, "following")
			}
		}
		for _, e := range fetchTypes.Elements {
			if e.Valid {
				g.FetchContentTypes = append(g.FetchContentTypes, e.String)
			} else {
				g.FetchContentTypes = append(g.FetchContentTypes, "NEVER")
			}
		}

		groups = append(groups, g)
	}
	return groups, rows.Err()
}
