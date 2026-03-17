package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/jackc/pgx/v5"
	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

// FindThumbnailData is the job payload for find-thumbnail.
type FindThumbnailData struct {
	LibraryItemID string `json:"libraryItemId"`
	UserID        string `json:"userId"`
}

// HandleFindThumbnail finds the best thumbnail for a library item.
func HandleFindThumbnail(ctx context.Context, cfg *config.Config, redisDS *redisutil.RedisDataSource, dbPool *db.Pool, data []byte) error {
	var d FindThumbnailData
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("find-thumbnail: unmarshal: %w", err)
	}

	// Step 1: Fetch current thumbnail and content in AuthTrx.
	var thumbnail, readableContent string
	var itemFound bool
	if err := dbPool.AuthTrx(ctx, d.UserID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT COALESCE(thumbnail, ''), COALESCE(readable_content, '')
			FROM omnivore.library_item
			WHERE id = $1 AND user_id = $2
		`, d.LibraryItemID, d.UserID).Scan(&thumbnail, &readableContent)
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		itemFound = true
		return nil
	}); err != nil {
		return fmt.Errorf("find-thumbnail: query: %w", err)
	}

	if !itemFound {
		log.Printf("[find-thumbnail] item=%s not found, skipping", d.LibraryItemID)
		return nil
	}

	// Step 2: HTTP processing (outside transaction).
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// If an existing thumbnail is set, verify it's still accessible.
	if thumbnail != "" {
		if !isImageAccessible(httpClient, thumbnail) {
			thumbnail = ""
		}
	}

	// If no valid thumbnail yet, scan readable_content for candidates.
	if thumbnail == "" {
		srcs := extractImgSrcs(readableContent)
		best := pickBestThumbnail(httpClient, srcs)
		if best != "" {
			thumbnail = best
		}
	}

	// Step 3: Update thumbnail in AuthTrx.
	if thumbnail != "" {
		if err := dbPool.AuthTrx(ctx, d.UserID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				UPDATE omnivore.library_item SET thumbnail = $1 WHERE id = $2
			`, thumbnail, d.LibraryItemID)
			return err
		}); err != nil {
			return fmt.Errorf("find-thumbnail: update thumbnail: %w", err)
		}
		log.Printf("[find-thumbnail] set thumbnail for item=%s url=%s", d.LibraryItemID, thumbnail)
	}
	return nil
}

func isImageAccessible(client *http.Client, url string) bool {
	resp, err := client.Head(url)
	if err != nil || resp.StatusCode >= 400 {
		return false
	}
	resp.Body.Close()
	return true
}

// extractImgSrcs parses HTML and collects all img src attribute values.
func extractImgSrcs(htmlContent string) []string {
	var srcs []string
	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			tok := tokenizer.Token()
			if tok.Data == "img" {
				for _, attr := range tok.Attr {
					if attr.Key == "src" && attr.Val != "" {
						srcs = append(srcs, attr.Val)
					}
				}
			}
		}
	}
	return srcs
}

type imgCandidate struct {
	url  string
	area float64
}

// pickBestThumbnail applies the thumbnail scoring algorithm from the TS implementation.
func pickBestThumbnail(client *http.Client, srcs []string) string {
	const maxBody = 20 * 1024 * 1024 // 20 MB

	var best imgCandidate
	for _, src := range srcs {
		if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
			continue
		}

		resp, err := client.Get(src)
		if err != nil || resp.StatusCode >= 400 {
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}

		cfg, _, err := image.DecodeConfig(io.LimitReader(resp.Body, maxBody))
		resp.Body.Close()
		if err != nil {
			continue
		}

		w := float64(cfg.Width)
		h := float64(cfg.Height)
		area := w * h

		// Skip tiny images.
		if area < 5000 {
			continue
		}

		// Penalise excessively wide/tall images.
		if w > 0 && h > 0 {
			ratio := math.Max(w/h, h/w)
			if ratio > 1.5 {
				area /= ratio * 2
			}
		}

		// Penalise sprite sheets.
		if strings.Contains(strings.ToLower(src), "sprite") {
			area /= 10
		}

		if area > best.area {
			best = imgCandidate{url: src, area: area}
		}
	}
	return best.url
}
