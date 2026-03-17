package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	readability "github.com/go-shiori/go-readability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/redisutil"
	"github.com/omnivore-app/omnivore/internal/storage"
)

// DBQuerier is the minimal database interface required by save-page.
// *db.Pool (which wraps *pgxpool.Pool) satisfies this interface.
type DBQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// SavePageData mirrors SavePageJobData from save_page.ts.
type SavePageData struct {
	UserID                 string   `json:"userId"`
	URL                    string   `json:"url"`
	FinalURL               string   `json:"finalUrl"`
	ArticleSavingRequestID string   `json:"articleSavingRequestId"`
	State                  *string  `json:"state,omitempty"`
	Labels                 []Label  `json:"labels,omitempty"`
	Source                 string   `json:"source"`
	Folder                 *string  `json:"folder,omitempty"`
	RSSFeedURL             *string  `json:"rssFeedUrl,omitempty"`
	SavedAt                *string  `json:"savedAt,omitempty"`
	PublishedAt            *string  `json:"publishedAt,omitempty"`
	Title                  string   `json:"title,omitempty"`
	ContentType            string   `json:"contentType,omitempty"`
	CacheKey               string   `json:"cacheKey,omitempty"`
	TaskID                 *string  `json:"taskId,omitempty"`
}

// Label mirrors CreateLabelInput.
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// cachePayload is the shape stored in Redis by the content-fetcher.
type cachePayload struct {
	FinalURL string `json:"finalUrl"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

// HandleSavePage processes a save-page job from the backend queue.
func HandleSavePage(
	ctx context.Context,
	cfg *config.Config,
	redisDS *redisutil.RedisDataSource,
	dbq DBQuerier,
	rawData json.RawMessage,
) error {
	var data SavePageData
	if err := json.Unmarshal(rawData, &data); err != nil {
		return fmt.Errorf("unmarshal save-page data: %w", err)
	}

	log.Printf("[save-page] userId=%s url=%s id=%s", data.UserID, data.URL, data.ArticleSavingRequestID)

	// PDF handling is not yet implemented; skip gracefully.
	if data.ContentType == "application/pdf" {
		log.Printf("[save-page] PDF content not yet supported, skipping id=%s", data.ArticleSavingRequestID)
		return nil
	}

	// 1. Fetch HTML content — cache first, then blob storage.
	htmlContent, finalURL, err := fetchContent(ctx, cfg, redisDS, &data)
	if err != nil {
		return fmt.Errorf("fetch content: %w", err)
	}
	if htmlContent == "" {
		return fmt.Errorf("no content available for id=%s", data.ArticleSavingRequestID)
	}

	// 2. Parse HTML with go-readability.
	parsedURL, err := url.Parse(finalURL)
	if err != nil {
		parsedURL, _ = url.Parse(data.URL)
	}

	article, err := readability.FromReader(strings.NewReader(htmlContent), parsedURL)
	if err != nil {
		log.Printf("[save-page] readability failed for id=%s: %v – using raw content", data.ArticleSavingRequestID, err)
	}

	title := data.Title
	if title == "" {
		title = article.Title
	}
	if title == "" {
		title = parsedURL.Host
	}

	wordCount := len(strings.Fields(article.TextContent))
	readableContent := article.Content
	author := article.Byline
	description := article.Excerpt
	siteName := parsedURL.Hostname()

	// 3. Determine state, folder, subscription.
	state := "SUCCEEDED"
	if data.State != nil && *data.State != "" {
		state = *data.State
	}

	folder := "inbox"
	if data.Folder != nil && *data.Folder != "" {
		folder = *data.Folder
	}

	var subscription *string
	if data.RSSFeedURL != nil && *data.RSSFeedURL != "" {
		subscription = data.RSSFeedURL
	}

	// 4. Parse timestamps.
	savedAt := time.Now()
	if data.SavedAt != nil && *data.SavedAt != "" {
		if t, err := time.Parse(time.RFC3339, *data.SavedAt); err == nil {
			savedAt = t
		}
	}

	var publishedAt *time.Time
	if data.PublishedAt != nil && *data.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, *data.PublishedAt); err == nil {
			publishedAt = &t
		}
	}

	slug := makeSlug(title)

	// 5. Upsert into omnivore.library_item.
	if err := upsertLibraryItem(ctx, dbq, libraryItem{
		ID:              data.ArticleSavingRequestID,
		UserID:          data.UserID,
		Slug:            slug,
		Title:           title,
		Author:          author,
		Description:     description,
		OriginalURL:     finalURL,
		ReadableContent: readableContent,
		WordCount:       wordCount,
		State:           state,
		SavedAt:         savedAt,
		PublishedAt:     publishedAt,
		ItemType:        "ARTICLE",
		ContentReader:   "WEB",
		Folder:          folder,
		Subscription:    subscription,
		SiteName:        siteName,
	}); err != nil {
		return fmt.Errorf("upsert library item: %w", err)
	}

	log.Printf("[save-page] saved id=%s title=%q words=%d", data.ArticleSavingRequestID, title, wordCount)
	return nil
}

func fetchContent(
	ctx context.Context,
	cfg *config.Config,
	redisDS *redisutil.RedisDataSource,
	data *SavePageData,
) (htmlContent, finalURL string, err error) {
	finalURL = data.FinalURL
	if finalURL == "" {
		finalURL = data.URL
	}

	// Try Redis cache first.
	if data.CacheKey != "" {
		raw, rerr := redisDS.CacheClient.Get(ctx, data.CacheKey).Result()
		if rerr == nil && raw != "" {
			var payload cachePayload
			if jerr := json.Unmarshal([]byte(raw), &payload); jerr == nil {
				if payload.FinalURL != "" {
					finalURL = payload.FinalURL
				}
				log.Printf("[save-page] content from cache key=%s", data.CacheKey)
				return payload.Content, finalURL, nil
			}
		}
		log.Printf("[save-page] cache miss or error for key=%s: %v", data.CacheKey, rerr)
	}

	// Fall back to blob storage.
	if cfg.BlobStorageURL == "" && cfg.GCSUploadBucket == "omnivore-files" {
		// No usable blob storage configured — skip rather than error.
		log.Printf("[save-page] no blob storage configured, skipping blob download for id=%s", data.ArticleSavingRequestID)
		return "", finalURL, nil
	}

	blobPath := contentBlobPath(data)
	storageClient, serr := storage.New(ctx, cfg.BlobURL())
	if serr != nil {
		return "", finalURL, fmt.Errorf("open storage: %w", serr)
	}
	defer storageClient.Close()

	content, serr := storageClient.DownloadContent(ctx, blobPath)
	if serr != nil {
		return "", finalURL, fmt.Errorf("download blob %s: %w", blobPath, serr)
	}
	log.Printf("[save-page] content from blob path=%s", blobPath)
	return content, finalURL, nil
}

// contentBlobPath mirrors contentFilePath() from uploads.ts.
// Path: content/{userId}/{libraryItemId}.{savedAtMs}.original
func contentBlobPath(data *SavePageData) string {
	savedAtMs := time.Now().UnixMilli()
	if data.SavedAt != nil && *data.SavedAt != "" {
		if t, err := time.Parse(time.RFC3339, *data.SavedAt); err == nil {
			savedAtMs = t.UnixMilli()
		}
	}
	return fmt.Sprintf("content/%s/%s.%d.original", data.UserID, data.ArticleSavingRequestID, savedAtMs)
}

// libraryItem holds the fields needed for the upsert.
type libraryItem struct {
	ID              string
	UserID          string
	Slug            string
	Title           string
	Author          string
	Description     string
	OriginalURL     string
	ReadableContent string
	WordCount       int
	State           string
	SavedAt         time.Time
	PublishedAt     *time.Time
	ItemType        string
	ContentReader   string
	Folder          string
	Subscription    *string
	SiteName        string
}

func upsertLibraryItem(ctx context.Context, dbq DBQuerier, item libraryItem) error {
	// Check if item exists.
	var existingID string
	err := dbq.QueryRow(ctx,
		`SELECT id FROM omnivore.library_item WHERE id = $1 AND user_id = $2`,
		item.ID, item.UserID,
	).Scan(&existingID)

	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("select library item: %w", err)
	}

	if err == pgx.ErrNoRows {
		// Insert new item.
		_, err = dbq.Exec(ctx, `
			INSERT INTO omnivore.library_item (
				id, user_id, slug, title, author, description,
				original_url, readable_content, word_count, state,
				saved_at, published_at, item_type, content_reader,
				folder, subscription, site_name
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				$7, $8, $9, $10,
				$11, $12, $13, $14,
				$15, $16, $17
			)`,
			item.ID, item.UserID, item.Slug, item.Title, nullStr(item.Author), nullStr(item.Description),
			item.OriginalURL, item.ReadableContent, item.WordCount, item.State,
			item.SavedAt, item.PublishedAt, item.ItemType, item.ContentReader,
			item.Folder, item.Subscription, nullStr(item.SiteName),
		)
		if err != nil {
			return fmt.Errorf("insert library item: %w", err)
		}
		return nil
	}

	// Update existing item.
	_, err = dbq.Exec(ctx, `
		UPDATE omnivore.library_item SET
			title = $3,
			author = $4,
			description = $5,
			readable_content = $6,
			word_count = $7,
			state = $8,
			published_at = $9,
			site_name = $10,
			updated_at = now()
		WHERE id = $1 AND user_id = $2`,
		item.ID, item.UserID,
		item.Title, nullStr(item.Author), nullStr(item.Description),
		item.ReadableContent, item.WordCount, item.State,
		item.PublishedAt, nullStr(item.SiteName),
	)
	if err != nil {
		return fmt.Errorf("update library item: %w", err)
	}
	return nil
}

// nullStr converts an empty string to nil for nullable DB columns.
func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9-]+`)

// makeSlug produces a URL-safe slug from a title.
func makeSlug(title string) string {
	slug := strings.ToLower(title)
	slug = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '-'
	}, slug)
	slug = nonAlphanumeric.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "untitled"
	}
	if len(slug) > 100 {
		slug = slug[:100]
	}
	return slug
}
