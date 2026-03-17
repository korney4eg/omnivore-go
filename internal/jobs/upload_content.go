package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"

	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/redisutil"
	"github.com/omnivore-app/omnivore/internal/storage"
)

// UploadContentData is the job payload for upload-content.
type UploadContentData struct {
	LibraryItemID string `json:"libraryItemId"`
	UserID        string `json:"userId"`
	Format        string `json:"format"`             // "markdown"|"highlightedMarkdown"|"original"|"readable"
	FilePath      string `json:"filePath"`
	Content       string `json:"content,omitempty"`
}

// HandleUploadContent fetches or uses provided content, converts by format, and uploads to blob storage.
func HandleUploadContent(ctx context.Context, cfg *config.Config, redisDS *redisutil.RedisDataSource, dbPool *db.Pool, data []byte) error {
	var d UploadContentData
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("upload-content: unmarshal: %w", err)
	}

	content := d.Content
	if content == "" {
		if err := dbPool.QueryRow(ctx, `
			SELECT COALESCE(readable_content, '')
			FROM omnivore.library_item
			WHERE id = $1 AND user_id = $2
		`, d.LibraryItemID, d.UserID).Scan(&content); err != nil {
			return fmt.Errorf("upload-content: query readable_content: %w", err)
		}
	}

	var contentType string
	switch strings.ToLower(d.Format) {
	case "markdown", "highlightedmarkdown":
		converted, err := convertToMarkdown(content)
		if err != nil {
			log.Printf("[upload-content] markdown conversion failed, using raw HTML: %v", err)
		} else {
			content = converted
		}
		contentType = "text/markdown"
	default:
		// "original" and "readable" stay as HTML
		contentType = "text/html"
	}

	blobURL := cfg.BlobURL()
	if blobURL == "" {
		log.Printf("[upload-content] no blob storage configured, skipping upload for item=%s", d.LibraryItemID)
		return nil
	}

	sc, err := storage.New(ctx, blobURL)
	if err != nil {
		return fmt.Errorf("upload-content: open storage: %w", err)
	}
	defer sc.Close()

	if err := sc.UploadBytes(ctx, d.FilePath, []byte(content), contentType); err != nil {
		return fmt.Errorf("upload-content: upload: %w", err)
	}

	log.Printf("[upload-content] uploaded item=%s format=%s path=%s", d.LibraryItemID, d.Format, d.FilePath)
	return nil
}

func convertToMarkdown(htmlContent string) (string, error) {
	converter := md.NewConverter("", true, nil)
	return converter.ConvertString(htmlContent)
}
