package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

// UpdateHighlightData is the job payload for update-highlight.
type UpdateHighlightData struct {
	LibraryItemID string `json:"libraryItemId"`
	UserID        string `json:"userId"`
}

// HandleUpdateHighlight syncs highlight_annotations on the library_item from the highlight table.
func HandleUpdateHighlight(ctx context.Context, cfg *config.Config, redisDS *redisutil.RedisDataSource, dbPool *db.Pool, data []byte) error {
	var d UpdateHighlightData
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("update-highlight: unmarshal: %w", err)
	}

	_, err := dbPool.Exec(ctx, `
		UPDATE omnivore.library_item
		SET highlight_annotations = COALESCE((
		  SELECT array_agg(COALESCE(annotation, ''))
		  FROM omnivore.highlight
		  WHERE library_item_id = $1
		), ARRAY[]::TEXT[])
		WHERE id = $1
	`, d.LibraryItemID)
	if err != nil {
		return fmt.Errorf("update-highlight: exec: %w", err)
	}

	log.Printf("[update-highlight] updated highlights for item=%s", d.LibraryItemID)
	return nil
}
