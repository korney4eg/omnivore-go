package jobs

import (
	"context"
	"fmt"
	"log"

	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

// HandleExpireFolders calls the stored procedure to expire overdue folders.
func HandleExpireFolders(ctx context.Context, cfg *config.Config, redisDS *redisutil.RedisDataSource, dbPool *db.Pool, data []byte) error {
	_, err := dbPool.Exec(ctx, `CALL omnivore.expire_folders()`)
	if err != nil {
		return fmt.Errorf("expire-folders: call procedure: %w", err)
	}

	log.Printf("[expire-folders] expired folders")
	return nil
}
