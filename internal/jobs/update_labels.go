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

// UpdateLabelsData is the job payload for update-labels.
type UpdateLabelsData struct {
	LibraryItemID string `json:"libraryItemId"`
	UserID        string `json:"userId"`
}

// HandleUpdateLabels syncs label_names on the library_item from the entity_labels join table.
func HandleUpdateLabels(ctx context.Context, cfg *config.Config, redisDS *redisutil.RedisDataSource, dbPool *db.Pool, data []byte) error {
	var d UpdateLabelsData
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("update-labels: unmarshal: %w", err)
	}

	_, err := dbPool.Exec(ctx, `
		UPDATE omnivore.library_item
		SET label_names = COALESCE((
		  SELECT array_agg(DISTINCT l.name)
		  FROM omnivore.labels l
		  INNER JOIN omnivore.entity_labels el
		    ON el.label_id = l.id
		    AND el.library_item_id = $1
		), ARRAY[]::TEXT[])
		WHERE id = $1
	`, d.LibraryItemID)
	if err != nil {
		return fmt.Errorf("update-labels: exec: %w", err)
	}

	log.Printf("[update-labels] updated labels for item=%s", d.LibraryItemID)
	return nil
}
