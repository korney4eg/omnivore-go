package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mmcdole/gofeed"
	"github.com/omnivore-app/omnivore/internal/bullmq"
	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

const (
	feedFetchFailureKeyPrefix  = "feed-fetch-failure:"
	recentSavedItemKeyPrefix   = "recent-saved-item:"
	feedContentCacheKeyPrefix  = "save-page-feed-content:"
	feedContentCacheTTL        = 24 * time.Hour
	feedFetchFailureExpiry     = 24 * 60 * 60 // seconds
	maxItemsPerFeed            = 100
	feedFetchTimeout           = 60 * time.Second
	feedFetchMaxRedirects      = 10
)

// feedDB is the minimal DB interface needed by HandleRefreshFeed.
type feedDB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// RefreshFeedRequest is the decoded payload for a refresh-feed job.
type RefreshFeedRequest struct {
	SubscriptionIDs      []string       `json:"subscriptionIds"`
	FeedURL              string         `json:"feedUrl"`
	MostRecentItemDates  []int64        `json:"mostRecentItemDates"`  // unix ms
	ScheduledTimestamps  []int64        `json:"scheduledTimestamps"`  // unix ms
	LastFetchedChecksums []string       `json:"lastFetchedChecksums"`
	UserIDs              []string       `json:"userIds"`
	FetchContentTypes    []string       `json:"fetchContentTypes"` // "ALWAYS"|"NEVER"|"WHEN_EMPTY"
	Folders              []string       `json:"folders"`           // "following"|"inbox"
	Priority             string         `json:"priority,omitempty"` // "low"|"high"
}

// contentFetchUser mirrors the UserConfig for the fetch-content job.
type contentFetchUser struct {
	ID            string `json:"id"`
	Folder        string `json:"folder"`
	LibraryItemID string `json:"libraryItemId"`
}

// contentFetchJobData is the payload for a fetch-content job on omnivore-content-fetch-queue.
type contentFetchJobData struct {
	URL         string             `json:"url"`
	Users       []contentFetchUser `json:"users"`
	Priority    string             `json:"priority,omitempty"`
	Source      string             `json:"source"`
	Labels      []Label            `json:"labels,omitempty"`
	RSSFeedURL  string             `json:"rssFeedUrl,omitempty"`
	SavedAt     string             `json:"savedAt,omitempty"`
	PublishedAt string             `json:"publishedAt,omitempty"`
}

// feedContentCache is what we store in Redis for the save-page cache path.
type feedContentCache struct {
	FinalURL string `json:"finalUrl"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

// HandleRefreshFeed processes a refresh-feed job.
func HandleRefreshFeed(
	ctx context.Context,
	cfg *config.Config,
	redisDS *redisutil.RedisDataSource,
	dbq feedDB,
	rawData json.RawMessage,
) error {
	var req RefreshFeedRequest
	if err := json.Unmarshal(rawData, &req); err != nil {
		return fmt.Errorf("unmarshal refresh-feed data: %w", err)
	}

	log.Printf("[refresh-feed] url=%s subscriptions=%d", req.FeedURL, len(req.SubscriptionIDs))

	// 1. Check if feed is blocked due to repeated failures.
	failKey := feedFetchFailureKeyPrefix + req.FeedURL
	if blocked, _ := isFeedBlocked(ctx, redisDS, failKey, cfg.MaxFeedFetchFailures); blocked {
		return fmt.Errorf("feed is blocked after too many failures: %s", req.FeedURL)
	}

	// 2. Fetch the feed with checksum.
	body, checksum, err := fetchFeedWithChecksum(req.FeedURL)
	if err != nil {
		log.Printf("[refresh-feed] fetch failed url=%s: %v", req.FeedURL, err)
		incrementFeedFailure(ctx, redisDS, failKey)
		markSubscriptionsFailed(ctx, dbq, req.SubscriptionIDs)
		return fmt.Errorf("fetch feed %s: %w", req.FeedURL, err)
	}

	// 3. Parse the feed.
	fp := gofeed.NewParser()
	feed, err := fp.ParseString(body)
	if err != nil {
		log.Printf("[refresh-feed] parse failed url=%s: %v", req.FeedURL, err)
		incrementFeedFailure(ctx, redisDS, failKey)
		markSubscriptionsFailed(ctx, dbq, req.SubscriptionIDs)
		return fmt.Errorf("parse feed %s: %w", req.FeedURL, err)
	}

	// 4. Check content-fetch blocklist.
	allowFetchContent := !isContentFetchBlocked(req.FeedURL)

	log.Printf("[refresh-feed] parsed feed title=%q items=%d allowFetch=%v", feed.Title, len(feed.Items), allowFetchContent)

	// fetchContentTasks groups by item URL for deduplication across subscriptions.
	fetchContentTasks := make(map[string]*contentFetchJobData) // item URL → job data

	// 5. Process each subscription.
	for i := range req.SubscriptionIDs {
		subID := req.SubscriptionIDs[i]
		userID := safeIdx(req.UserIDs, i)
		mostRecentMs := safeInt64(req.MostRecentItemDates, i)
		scheduledMs := safeInt64(req.ScheduledTimestamps, i)
		lastChecksum := safeStr(req.LastFetchedChecksums, i)
		fetchType := safeStr(req.FetchContentTypes, i)
		folder := safeStr(req.Folders, i)
		if folder == "" {
			folder = "following"
		}

		if !allowFetchContent {
			fetchType = "NEVER"
		}

		if err := processSubscription(
			ctx, cfg, redisDS, dbq,
			fetchContentTasks,
			subID, userID, req.FeedURL,
			body, checksum, mostRecentMs, scheduledMs,
			lastChecksum, fetchType, folder, feed,
		); err != nil {
			log.Printf("[refresh-feed] subscription %s failed: %v", subID, err)
		}
	}

	// 6. Enqueue all accumulated content-fetch tasks.
	if len(fetchContentTasks) > 0 {
		if err := enqueueFetchContentTasks(ctx, redisDS, fetchContentTasks, req.Priority); err != nil {
			log.Printf("[refresh-feed] enqueue fetch-content tasks failed: %v", err)
		}
	}

	return nil
}

func processSubscription(
	ctx context.Context,
	cfg *config.Config,
	redisDS *redisutil.RedisDataSource,
	dbq feedDB,
	fetchContentTasks map[string]*contentFetchJobData,
	subID, userID, feedURL string,
	body, checksum string,
	mostRecentMs, scheduledMs int64,
	lastChecksum, fetchType, folder string,
	feed *gofeed.Feed,
) error {
	refreshedAt := time.Now()

	// Skip if checksum unchanged.
	if checksum == lastChecksum {
		log.Printf("[refresh-feed] subscription %s: feed unchanged (checksum match)", subID)
		return nil
	}

	// Skip if feed's build date is not newer than what we last saw.
	feedBuildDate := feed.UpdatedParsed
	if feedBuildDate == nil {
		feedBuildDate = feed.PublishedParsed
	}
	if feedBuildDate != nil && mostRecentMs > 0 && !feedBuildDate.After(time.UnixMilli(mostRecentMs)) {
		log.Printf("[refresh-feed] subscription %s: feed build date older than most recent item, skipping", subID)
		return nil
	}

	var lastItemFetchedAt *time.Time
	var lastValidItem *gofeed.Item
	var failedAt *time.Time
	itemCount := 0

	for _, item := range feed.Items {
		link := extractItemLink(item)
		if link == "" {
			link = item.GUID
		}
		if link == "" {
			continue
		}

		isoDate := itemISODate(item)

		feedItem := &gofeed.Item{}
		*feedItem = *item
		if feedItem.Link == "" {
			feedItem.Link = link
		}

		publishedAt := time.Now()
		if isoDate != "" {
			if t, err := time.Parse(time.RFC3339, isoDate); err == nil {
				publishedAt = t
			} else if t2, err2 := time.Parse("Mon, 02 Jan 2006 15:04:05 -0700", isoDate); err2 == nil {
				publishedAt = t2
			}
		}
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			publishedAt = *item.UpdatedParsed
		}

		// Track the most recent valid item for last-ditch saving.
		if lastValidItem == nil || (lastValidItem.PublishedParsed != nil && publishedAt.After(*lastValidItem.PublishedParsed)) {
			lastValidItem = feedItem
		}

		// Hard cap per feed.
		if itemCount >= maxItemsPerFeed {
			if itemCount == maxItemsPerFeed {
				log.Printf("[refresh-feed] max items (%d) reached for feed %s", maxItemsPerFeed, feedURL)
			}
			itemCount++
			continue
		}

		// Skip old items.
		if IsOldItem(item, mostRecentMs) {
			continue
		}

		// Skip recently saved items.
		recentKey := recentSavedItemKeyPrefix + userID + ":" + link
		if exists, _ := redisDS.CacheClient.Exists(ctx, recentKey).Result(); exists > 0 {
			log.Printf("[refresh-feed] item recently saved, skipping: %s", link)
			continue
		}

		// Decide action.
		feedContent := item.Content
		if feedContent == "" {
			feedContent = item.Description
		}
		var saveErr error
		if fetchType == "NEVER" || (fetchType == "WHEN_EMPTY" && feedContent != "") {
			saveErr = enqueueSavePageFromFeed(ctx, cfg, redisDS, userID, feedURL, link, item, isoDate, folder, feedContent)
		} else {
			// ALWAYS or WHEN_EMPTY without content → content fetch.
			accumulateFetchContentTask(fetchContentTasks, userID, folder, link, feedURL, isoDate, item)
		}

		if saveErr != nil {
			log.Printf("[refresh-feed] save item failed %s: %v", link, saveErr)
			now := time.Now()
			failedAt = &now
		} else {
			if lastItemFetchedAt == nil || publishedAt.After(*lastItemFetchedAt) {
				t := publishedAt
				lastItemFetchedAt = &t
			}
			itemCount++
		}
	}

	// If nothing was saved and no failure, try saving the last valid item for new subscriptions.
	if lastItemFetchedAt == nil && failedAt == nil && lastValidItem != nil && mostRecentMs == 0 {
		link := extractItemLink(lastValidItem)
		if link == "" {
			link = lastValidItem.GUID
		}
		if link != "" {
			isoDate := itemISODate(lastValidItem)
			feedContent := lastValidItem.Content
			if feedContent == "" {
				feedContent = lastValidItem.Description
			}
			if fetchType == "NEVER" || (fetchType == "WHEN_EMPTY" && feedContent != "") {
				if err := enqueueSavePageFromFeed(ctx, cfg, redisDS, userID, feedURL, link, lastValidItem, isoDate, folder, feedContent); err != nil {
					log.Printf("[refresh-feed] fallback save failed: %v", err)
					now := time.Now()
					failedAt = &now
				}
			} else {
				accumulateFetchContentTask(fetchContentTasks, userID, folder, link, feedURL, isoDate, lastValidItem)
			}

			if failedAt == nil {
				if lastValidItem.PublishedParsed != nil {
					t := *lastValidItem.PublishedParsed
					lastItemFetchedAt = &t
				} else {
					t := refreshedAt
					lastItemFetchedAt = &t
				}
			}
		}
	}

	// Update subscription in DB.
	updatePeriodMs := getUpdatePeriodInHours(feed) * 60 * 60 * 1000
	updateFreq := getUpdateFrequency(feed)
	nextScheduledMs := scheduledMs + int64(updatePeriodMs)*int64(updateFreq)

	if err := updateSubscription(ctx, dbq, subID, lastItemFetchedAt, checksum, nextScheduledMs, refreshedAt, failedAt); err != nil {
		log.Printf("[refresh-feed] update subscription %s failed: %v", subID, err)
	}

	return nil
}

// IsOldItem reports whether an item should be skipped because it's old.
// Mirrors isOldItem() from refreshFeed.ts.
// Exported for unit testing.
func IsOldItem(item *gofeed.Item, mostRecentItemTimestampMs int64) bool {
	var publishedAt *time.Time
	if item.PublishedParsed != nil {
		publishedAt = item.PublishedParsed
	} else if item.UpdatedParsed != nil {
		publishedAt = item.UpdatedParsed
	}

	// Always fetch items without a date.
	if publishedAt == nil {
		return false
	}

	if mostRecentItemTimestampMs == 0 {
		// New subscription: skip items older than 24h.
		return publishedAt.Before(time.Now().Add(-24 * time.Hour))
	}

	// Existing subscription: skip items at or before last seen date.
	return !publishedAt.After(time.UnixMilli(mostRecentItemTimestampMs))
}

// isContentFetchBlocked reports whether content fetching should be skipped for
// feeds from known heavy-traffic or non-web sources.
// Mirrors isContentFetchBlocked() from refreshFeed.ts.
func isContentFetchBlocked(feedURL string) bool {
	blocked := []string{
		"https://arxiv.org/",
		"https://rss.arxiv.org",
		"https://rsshub.app",
		"https://xkcd.com",
		"https://daringfireball.net/feeds/",
		"https://lwn.net/headlines/newrss",
		"https://medium.com",
	}
	for _, prefix := range blocked {
		if strings.HasPrefix(feedURL, prefix) {
			return true
		}
	}
	return false
}

func isFeedBlocked(ctx context.Context, redisDS *redisutil.RedisDataSource, key string, maxFailures int) (bool, error) {
	val, err := redisDS.CacheClient.Get(ctx, key).Result()
	if err != nil {
		return false, nil // assume not blocked on Redis error
	}
	var count int
	fmt.Sscanf(val, "%d", &count)
	return count > maxFailures, nil
}

func incrementFeedFailure(ctx context.Context, redisDS *redisutil.RedisDataSource, key string) {
	pipe := redisDS.CacheClient.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, time.Duration(feedFetchFailureExpiry)*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("[refresh-feed] incr feed failure key=%s: %v", key, err)
	}
}

func markSubscriptionsFailed(ctx context.Context, dbq feedDB, subIDs []string) {
	now := time.Now()
	for _, id := range subIDs {
		_, err := dbq.Exec(ctx, `
			UPDATE omnivore.subscriptions SET refreshed_at = $1, failed_at = $2 WHERE id = $3`,
			now, now, id)
		if err != nil {
			log.Printf("[refresh-feed] markSubscriptionsFailed id=%s: %v", id, err)
		}
	}
}

func fetchFeedWithChecksum(feedURL string) (body string, checksum string, err error) {
	client := &http.Client{
		Timeout: feedFetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= feedFetchMaxRedirects {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequest(http.MethodGet, feedURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "OmnivoreBot/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	h := sha256.Sum256(data)
	return string(data), fmt.Sprintf("%x", h[:]), nil
}

// extractItemLink picks the best link from a feed item.
// gofeed normalises links into item.Link for the primary link.
func extractItemLink(item *gofeed.Item) string {
	if item.Link != "" {
		return item.Link
	}
	if len(item.Links) > 0 {
		return item.Links[0]
	}
	return ""
}

// itemISODate returns the best ISO-8601 date string for a feed item.
func itemISODate(item *gofeed.Item) string {
	if item.PublishedParsed != nil {
		return item.PublishedParsed.UTC().Format(time.RFC3339)
	}
	if item.UpdatedParsed != nil {
		return item.UpdatedParsed.UTC().Format(time.RFC3339)
	}
	return ""
}

func enqueueSavePageFromFeed(
	ctx context.Context,
	cfg *config.Config,
	redisDS *redisutil.RedisDataSource,
	userID, feedURL, itemURL string,
	item *gofeed.Item,
	isoDate, folder, feedContent string,
) error {
	libraryItemID := uuid.New().String()
	cacheKey := feedContentCacheKeyPrefix + libraryItemID

	// Store feed content in Redis so save-page handler can retrieve it.
	cachePayload := feedContentCache{
		FinalURL: itemURL,
		Title:    item.Title,
		Content:  feedContent,
	}
	cacheBytes, err := json.Marshal(cachePayload)
	if err != nil {
		return fmt.Errorf("marshal cache payload: %w", err)
	}
	if err := redisDS.CacheClient.Set(ctx, cacheKey, string(cacheBytes), feedContentCacheTTL).Err(); err != nil {
		return fmt.Errorf("set cache key %s: %w", cacheKey, err)
	}

	state := "CONTENT_NOT_FETCHED"
	rssFeed := feedURL
	savedAt := isoDate
	publishedAt := isoDate
	folderVal := folder

	author := ""
	if item.Author != nil {
		author = item.Author.Name
	}

	data := SavePageData{
		UserID:                 userID,
		URL:                    itemURL,
		ArticleSavingRequestID: libraryItemID,
		State:                  &state,
		Labels:                 []Label{{Name: "RSS"}},
		Source:                 "rss-feeder",
		Folder:                 &folderVal,
		RSSFeedURL:             &rssFeed,
		SavedAt:                &savedAt,
		PublishedAt:            &publishedAt,
		Title:                  item.Title,
		Author:                 author,
		CacheKey:               cacheKey,
	}

	return enqueueSavePageJob(ctx, redisDS, data)
}

func enqueueSavePageJob(ctx context.Context, redisDS *redisutil.RedisDataSource, data SavePageData) error {
	jobs := []bullmq.AddJobOpts{{
		Name: bullmq.SavePageJob,
		Data: data,
		Opts: bullmq.JobOpts{
			Attempts: 3,
			Backoff:  bullmq.BackoffOpt{Type: "exponential", Delay: 1000},
		},
	}}
	return bullmq.AddBulk(ctx, redisDS.MQClient, bullmq.BackendQueue, jobs)
}

// accumulateFetchContentTask adds a user to a content-fetch task, grouping by item URL.
func accumulateFetchContentTask(
	tasks map[string]*contentFetchJobData,
	userID, folder, itemURL, feedURL, isoDate string,
	item *gofeed.Item,
) {
	task, exists := tasks[itemURL]
	if !exists {
		task = &contentFetchJobData{
			URL:         itemURL,
			Priority:    "low",
			Source:      "rss-feeder",
			Labels:      []Label{{Name: "RSS"}},
			RSSFeedURL:  feedURL,
			SavedAt:     isoDate,
			PublishedAt: isoDate,
		}
		tasks[itemURL] = task
	}
	task.Users = append(task.Users, contentFetchUser{
		ID:            userID,
		Folder:        folder,
		LibraryItemID: uuid.New().String(),
	})
	_ = item // kept for potential future use (e.g. thumbnail)
}

func enqueueFetchContentTasks(
	ctx context.Context,
	redisDS *redisutil.RedisDataSource,
	tasks map[string]*contentFetchJobData,
	priority string,
) error {
	if priority == "" {
		priority = "low"
	}

	var jobOpts []bullmq.AddJobOpts
	for itemURL, task := range tasks {
		task.Priority = priority

		// Deterministic job ID: fetch-content_<sha256(sortedPayloadJSON)>_v1
		sortedUsers := make([]contentFetchUser, len(task.Users))
		copy(sortedUsers, task.Users)
		sort.Slice(sortedUsers, func(i, j int) bool {
			return sortedUsers[i].ID < sortedUsers[j].ID
		})

		dedupPayload := struct {
			URL   string             `json:"url"`
			Users []contentFetchUser `json:"users"`
		}{URL: itemURL, Users: sortedUsers}
		dedupBytes, _ := json.Marshal(dedupPayload)
		jobID := fmt.Sprintf("fetch-content_%s_v1", sha256hex16(string(dedupBytes)))

		jobOpts = append(jobOpts, bullmq.AddJobOpts{
			Name:  "fetch-content",
			Data:  task,
			JobID: jobID,
			Opts: bullmq.JobOpts{
				Attempts: 3,
				Backoff:  bullmq.BackoffOpt{Type: "exponential", Delay: 1000},
			},
		})
	}

	return bullmq.AddBulk(ctx, redisDS.MQClient, bullmq.ContentFetchQueue, jobOpts)
}

func updateSubscription(
	ctx context.Context,
	dbq feedDB,
	subID string,
	lastItemFetchedAt *time.Time,
	checksum string,
	nextScheduledMs int64,
	refreshedAt time.Time,
	failedAt *time.Time,
) error {
	nextScheduled := time.UnixMilli(nextScheduledMs)
	_, err := dbq.Exec(ctx, `
		UPDATE omnivore.subscriptions SET
			most_recent_item_date = $1,
			last_fetched_checksum = $2,
			scheduled_at          = $3,
			refreshed_at          = $4,
			failed_at             = $5
		WHERE id = $6`,
		lastItemFetchedAt, checksum, nextScheduled, refreshedAt, failedAt, subID,
	)
	return err
}

// getUpdatePeriodInHours returns the feed update period in hours based on
// sy:updatePeriod extension field, defaulting to 1 (hourly).
func getUpdatePeriodInHours(feed *gofeed.Feed) int64 {
	period := extensionValue(feed, "sy", "updatePeriod")
	switch strings.ToLower(period) {
	case "daily":
		return 24
	case "weekly":
		return 7 * 24
	case "monthly":
		return 30 * 24
	case "yearly":
		return 365 * 24
	default:
		return 1
	}
}

// getUpdateFrequency returns the feed update frequency multiplier based on
// sy:updateFrequency extension field, defaulting to 1.
func getUpdateFrequency(feed *gofeed.Feed) int64 {
	v := extensionValue(feed, "sy", "updateFrequency")
	if v == "" {
		return 1
	}
	var f int64
	if _, err := fmt.Sscanf(v, "%d", &f); err != nil || f <= 0 {
		return 1
	}
	return f
}

// extensionValue retrieves a feed extension value by namespace and element name.
func extensionValue(feed *gofeed.Feed, ns, element string) string {
	if feed.Extensions == nil {
		return ""
	}
	nsMap, ok := feed.Extensions[ns]
	if !ok {
		return ""
	}
	elems, ok := nsMap[element]
	if !ok || len(elems) == 0 {
		return ""
	}
	return elems[0].Value
}

// safeIdx returns userIDs[i] or "" if out of range.
func safeIdx(ss []string, i int) string {
	if i < len(ss) {
		return ss[i]
	}
	return ""
}

// safeStr returns ss[i] or "" if out of range.
func safeStr(ss []string, i int) string {
	return safeIdx(ss, i)
}

// safeInt64 returns ss[i] or 0 if out of range.
func safeInt64(ss []int64, i int) int64 {
	if i < len(ss) {
		return ss[i]
	}
	return 0
}
