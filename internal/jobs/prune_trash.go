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

// PruneTrashData is the job payload for prune-trash.
type PruneTrashData struct {
	NumDays int `json:"numDays"`
}

// HandlePruneTrash calls the stored procedure to batch-delete trash items older than numDays.
func HandlePruneTrash(ctx context.Context, cfg *config.Config, redisDS *redisutil.RedisDataSource, dbPool *db.Pool, data []byte) error {
	var d PruneTrashData
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("prune-trash: unmarshal: %w", err)
	}

	numDays := d.NumDays
	if numDays == 0 {
		numDays = 30
	}

	_, err := dbPool.Exec(ctx, `CALL omnivore.batch_delete_trash_items($1)`, numDays)
	if err != nil {
		return fmt.Errorf("prune-trash: call procedure: %w", err)
	}

	log.Printf("[prune-trash] pruned trash items older than %d days", numDays)
	return nil
}
