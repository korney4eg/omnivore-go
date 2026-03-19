package resolver

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
	"github.com/omnivore-app/omnivore/internal/bullmq"
	"github.com/omnivore-app/omnivore/internal/graphql/model"
	dbmodel "github.com/omnivore-app/omnivore/internal/model"
	"github.com/redis/go-redis/v9"
)

type refreshFeedPayload struct {
	SubscriptionIDs      []string              `json:"subscriptionIds"`
	FeedURL              string                `json:"feedUrl"`
	MostRecentItemDates  []int64               `json:"mostRecentItemDates"`
	ScheduledTimestamps  []int64               `json:"scheduledTimestamps"`
	LastFetchedChecksums []string              `json:"lastFetchedChecksums"`
	UserIDs              []string              `json:"userIds"`
	FetchContentTypes    []string              `json:"fetchContentTypes"`
	Folders              []string              `json:"folders"`
	RefreshContext       refreshContextPayload `json:"refreshContext"`
}

type refreshContextPayload struct {
	Type      string `json:"type"`
	RefreshID string `json:"refreshID"`
	StartedAt string `json:"startedAt"`
}

func validateSubscriptionURL(raw string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported scheme")
	}
	if parsed.Host == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}

func canonicalFeedURL(inputURL string, feed *gofeed.Feed) string {
	if feed == nil {
		return inputURL
	}
	for _, candidate := range []string{feed.FeedLink, feed.Link} {
		if candidate == "" {
			continue
		}
		if err := validateSubscriptionURL(candidate); err == nil {
			return candidate
		}
	}
	return inputURL
}

func subscribeFeedName(feed *gofeed.Feed, feedURL string) string {
	if feed != nil && strings.TrimSpace(feed.Title) != "" {
		return strings.TrimSpace(feed.Title)
	}
	return feedURL
}

func subscribeFeedIcon(feed *gofeed.Feed) *string {
	if feed != nil && feed.Image != nil && strings.TrimSpace(feed.Image.URL) != "" {
		return strPtr(strings.TrimSpace(feed.Image.URL))
	}
	return nil
}

func emptyStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return strPtr(trimmed)
}

func subscribeFetchContentType(fetchType *model.FetchContentType) string {
	if fetchType == nil {
		return string(model.FetchContentTypeAlways)
	}
	return string(*fetchType)
}

func subscribeFetchContentValue(fetchContent *bool, fetchContentType string) bool {
	if fetchContent != nil {
		return *fetchContent
	}
	return strings.ToUpper(fetchContentType) != string(model.FetchContentTypeNever)
}

func enqueueRefreshFeed(ctx context.Context, redisClient *redis.Client, userID uuid.UUID, subscription *dbmodel.Subscription, feedURL string) error {
	mostRecentItemDate := int64(0)
	if subscription.MostRecentItemDate != nil {
		mostRecentItemDate = subscription.MostRecentItemDate.UTC().UnixMilli()
	}

	lastChecksum := ""
	if subscription.LastFetchedChecksum != nil {
		lastChecksum = *subscription.LastFetchedChecksum
	}

	startedAt := time.Now().UTC()
	payload := refreshFeedPayload{
		SubscriptionIDs:      []string{subscription.ID.String()},
		FeedURL:              feedURL,
		MostRecentItemDates:  []int64{mostRecentItemDate},
		ScheduledTimestamps:  []int64{startedAt.UnixMilli()},
		LastFetchedChecksums: []string{lastChecksum},
		UserIDs:              []string{userID.String()},
		FetchContentTypes:    []string{strings.ToUpper(subscription.FetchContentType)},
		Folders:              []string{subscription.Folder},
		RefreshContext: refreshContextPayload{
			Type:      "manual",
			RefreshID: uuid.New().String(),
			StartedAt: startedAt.Format(time.RFC3339),
		},
	}

	jobID := refreshFeedJobID(feedURL, []string{userID.String()})
	return bullmq.AddBulk(ctx, redisClient, bullmq.BackendQueue, []bullmq.AddJobOpts{{
		Name:  "refresh-feed",
		Data:  payload,
		JobID: jobID,
		Opts: bullmq.JobOpts{
			Attempts: 3,
			Backoff:  bullmq.BackoffOpt{Type: "exponential", Delay: 1000},
		},
	}})
}

func refreshFeedJobID(feedURL string, userIDs []string) string {
	sortedUserIDs := append([]string(nil), userIDs...)
	sort.Strings(sortedUserIDs)
	userBytes, _ := json.Marshal(sortedUserIDs)

	urlHash := sha256.Sum256([]byte(feedURL))
	userHash := sha256.Sum256(userBytes)
	return fmt.Sprintf("refresh-feed_%x_%x", urlHash[:8], userHash[:8])
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
