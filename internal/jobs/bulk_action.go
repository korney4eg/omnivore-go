package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

// BulkActionData is the job payload for bulk-action.
type BulkActionData struct {
	Count     int             `json:"count"`
	UserID    string          `json:"userId"`
	Action    string          `json:"action"`
	Query     string          `json:"query"`
	BatchSize int             `json:"batchSize"`
	LabelIDs  []string        `json:"labelIds,omitempty"`
	Args      json.RawMessage `json:"args,omitempty"`
}

// HandleBulkAction applies a batch action to library items matching a search query.
func HandleBulkAction(ctx context.Context, cfg *config.Config, redisDS *redisutil.RedisDataSource, dbPool *db.Pool, data []byte) error {
	var d BulkActionData
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("bulk-action: unmarshal: %w", err)
	}

	batchSize := d.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	total := d.Count
	if total <= 0 {
		total = batchSize
	}

	filter := parseBulkFilter(d.Query)

	switch d.Action {
	case "ARCHIVE":
		return bulkArchive(ctx, dbPool, d.UserID, filter, batchSize, total)
	case "DELETE":
		return bulkDelete(ctx, dbPool, d.UserID, filter, batchSize, total)
	case "MARK_AS_READ":
		return bulkMarkAsRead(ctx, dbPool, d.UserID, filter, batchSize, total)
	case "MARK_AS_SEEN":
		return bulkMarkAsSeen(ctx, dbPool, d.UserID, filter, batchSize, total)
	case "ADD_LABELS":
		return bulkAddLabels(ctx, dbPool, d.UserID, filter, batchSize, total, d.LabelIDs)
	case "MOVE_TO_FOLDER":
		folder, err := parseFolderArg(d.Args)
		if err != nil {
			return fmt.Errorf("bulk-action: parse folder arg: %w", err)
		}
		return bulkMoveToFolder(ctx, dbPool, d.UserID, filter, batchSize, total, folder)
	default:
		log.Printf("[bulk-action] unknown action %q, skipping", d.Action)
		return nil
	}
}

// bulkFilter holds a parsed representation of the search query.
type bulkFilter struct {
	stateFilter  string // "ACTIVE", "ARCHIVED", "DELETED", or "" for no filter
	folderFilter string // folder name or ""
	textSearch   string // title/description ILIKE
	labelName    string // label filter
}

// parseBulkFilter extracts the most common BullMQ query tokens.
func parseBulkFilter(query string) bulkFilter {
	var f bulkFilter
	terms := strings.Fields(query)
	var remaining []string
	for _, t := range terms {
		switch {
		case strings.HasPrefix(t, "in:"):
			switch strings.TrimPrefix(t, "in:") {
			case "inbox":
				f.folderFilter = "inbox"
			case "archive":
				f.stateFilter = "ARCHIVED"
			case "trash":
				f.stateFilter = "DELETED"
			case "following":
				f.folderFilter = "following"
			// "in:all" — no filter
			}
		case strings.HasPrefix(t, "label:"):
			f.labelName = strings.TrimPrefix(t, "label:")
		default:
			remaining = append(remaining, t)
		}
	}
	f.textSearch = strings.Join(remaining, " ")
	return f
}

// filterClause returns additional SQL fragments (appended after WHERE user_id = $1).
// args starts at index 2 ($2, $3, …).
func filterClause(f bulkFilter, args []any) (string, []any) {
	var parts []string
	n := len(args) + 1 // next placeholder index

	if f.stateFilter != "" {
		parts = append(parts, fmt.Sprintf("AND state = $%d", n))
		args = append(args, f.stateFilter)
		n++
	} else {
		parts = append(parts, "AND state NOT IN ('DELETED')")
	}

	if f.folderFilter != "" {
		parts = append(parts, fmt.Sprintf("AND folder = $%d", n))
		args = append(args, f.folderFilter)
		n++
	}

	if f.labelName != "" {
		parts = append(parts, fmt.Sprintf("AND $%d = ANY(label_names)", n))
		args = append(args, f.labelName)
		n++
	}

	if f.textSearch != "" {
		parts = append(parts, fmt.Sprintf("AND (title ILIKE $%d OR description ILIKE $%d)", n, n))
		args = append(args, "%"+f.textSearch+"%")
		n++
	}
	_ = n

	return strings.Join(parts, " "), args
}

func bulkArchive(ctx context.Context, dbPool *db.Pool, userID string, f bulkFilter, batchSize, total int) error {
	processed := 0
	for processed < total {
		limit := batchSize
		if processed+limit > total {
			limit = total - processed
		}
		args := []any{userID}
		extra, args := filterClause(f, args)
		n := len(args) + 1
		args = append(args, limit)
		sql := fmt.Sprintf(`
			UPDATE omnivore.library_item
			SET state = 'ARCHIVED', archived_at = now(), updated_at = now()
			WHERE id IN (
			  SELECT id FROM omnivore.library_item
			  WHERE user_id = $1 %s AND state NOT IN ('ARCHIVED','DELETED')
			  LIMIT $%d
			)`, extra, n)
		var rows int64
		if err := dbPool.AuthTrx(ctx, userID, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, sql, args...)
			if err != nil {
				return err
			}
			rows = tag.RowsAffected()
			return nil
		}); err != nil {
			return fmt.Errorf("bulk-archive: %w", err)
		}
		processed += int(rows)
		if rows == 0 {
			break
		}
	}
	log.Printf("[bulk-action] archived %d items for user=%s", processed, userID)
	return nil
}

func bulkDelete(ctx context.Context, dbPool *db.Pool, userID string, f bulkFilter, batchSize, total int) error {
	processed := 0
	for processed < total {
		limit := batchSize
		if processed+limit > total {
			limit = total - processed
		}
		args := []any{userID}
		extra, args := filterClause(f, args)
		n := len(args) + 1
		args = append(args, limit)
		sql := fmt.Sprintf(`
			UPDATE omnivore.library_item
			SET state = 'DELETED', deleted_at = now(), updated_at = now()
			WHERE id IN (
			  SELECT id FROM omnivore.library_item
			  WHERE user_id = $1 %s AND state != 'DELETED'
			  LIMIT $%d
			)`, extra, n)
		var rows int64
		if err := dbPool.AuthTrx(ctx, userID, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, sql, args...)
			if err != nil {
				return err
			}
			rows = tag.RowsAffected()
			return nil
		}); err != nil {
			return fmt.Errorf("bulk-delete: %w", err)
		}
		processed += int(rows)
		if rows == 0 {
			break
		}
	}
	log.Printf("[bulk-action] deleted %d items for user=%s", processed, userID)
	return nil
}

func bulkMarkAsRead(ctx context.Context, dbPool *db.Pool, userID string, f bulkFilter, batchSize, total int) error {
	processed := 0
	for processed < total {
		limit := batchSize
		if processed+limit > total {
			limit = total - processed
		}
		args := []any{userID}
		extra, args := filterClause(f, args)
		n := len(args) + 1
		args = append(args, limit)
		sql := fmt.Sprintf(`
			UPDATE omnivore.library_item
			SET reading_progress_bottom_percent = 100,
			    reading_progress_top_percent = 100,
			    read_at = now()
			WHERE id IN (
			  SELECT id FROM omnivore.library_item
			  WHERE user_id = $1 %s
			  LIMIT $%d
			)`, extra, n)
		var rows int64
		if err := dbPool.AuthTrx(ctx, userID, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, sql, args...)
			if err != nil {
				return err
			}
			rows = tag.RowsAffected()
			return nil
		}); err != nil {
			return fmt.Errorf("bulk-mark-as-read: %w", err)
		}
		processed += int(rows)
		if rows == 0 {
			break
		}
	}
	log.Printf("[bulk-action] marked-as-read %d items for user=%s", processed, userID)
	return nil
}

func bulkMarkAsSeen(ctx context.Context, dbPool *db.Pool, userID string, f bulkFilter, batchSize, total int) error {
	processed := 0
	for processed < total {
		limit := batchSize
		if processed+limit > total {
			limit = total - processed
		}
		args := []any{userID}
		extra, args := filterClause(f, args)
		n := len(args) + 1
		args = append(args, limit)
		sql := fmt.Sprintf(`
			UPDATE omnivore.library_item
			SET seen_at = now()
			WHERE id IN (
			  SELECT id FROM omnivore.library_item
			  WHERE user_id = $1 %s
			  LIMIT $%d
			)`, extra, n)
		var rows int64
		if err := dbPool.AuthTrx(ctx, userID, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, sql, args...)
			if err != nil {
				return err
			}
			rows = tag.RowsAffected()
			return nil
		}); err != nil {
			return fmt.Errorf("bulk-mark-as-seen: %w", err)
		}
		processed += int(rows)
		if rows == 0 {
			break
		}
	}
	log.Printf("[bulk-action] marked-as-seen %d items for user=%s", processed, userID)
	return nil
}

func bulkMoveToFolder(ctx context.Context, dbPool *db.Pool, userID string, f bulkFilter, batchSize, total int, folder string) error {
	processed := 0
	for processed < total {
		limit := batchSize
		if processed+limit > total {
			limit = total - processed
		}
		args := []any{userID}
		extra, args := filterClause(f, args)
		nFolder := len(args) + 1
		args = append(args, folder)
		nLimit := nFolder + 1
		args = append(args, limit)
		sql := fmt.Sprintf(`
			UPDATE omnivore.library_item
			SET folder = $%d, updated_at = now()
			WHERE id IN (
			  SELECT id FROM omnivore.library_item
			  WHERE user_id = $1 %s
			  LIMIT $%d
			)`, nFolder, extra, nLimit)
		var rows int64
		if err := dbPool.AuthTrx(ctx, userID, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, sql, args...)
			if err != nil {
				return err
			}
			rows = tag.RowsAffected()
			return nil
		}); err != nil {
			return fmt.Errorf("bulk-move-to-folder: %w", err)
		}
		processed += int(rows)
		if rows == 0 {
			break
		}
	}
	log.Printf("[bulk-action] moved %d items to folder=%s for user=%s", processed, folder, userID)
	return nil
}

func bulkAddLabels(ctx context.Context, dbPool *db.Pool, userID string, f bulkFilter, batchSize, total int, labelIDs []string) error {
	if len(labelIDs) == 0 {
		return nil
	}
	processed := 0
	for processed < total {
		limit := batchSize
		if processed+limit > total {
			limit = total - processed
		}
		args := []any{userID}
		extra, args := filterClause(f, args)
		n := len(args) + 1
		args = append(args, limit)
		selectSQL := fmt.Sprintf(`
			SELECT id FROM omnivore.library_item
			WHERE user_id = $1 %s
			LIMIT $%d`, extra, n)

		var count int
		if err := dbPool.AuthTrx(ctx, userID, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, selectSQL, args...)
			if err != nil {
				return fmt.Errorf("query items: %w", err)
			}
			var itemIDs []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return fmt.Errorf("scan id: %w", err)
				}
				itemIDs = append(itemIDs, id)
			}
			rows.Close()
			if rows.Err() != nil {
				return fmt.Errorf("rows: %w", rows.Err())
			}
			for _, itemID := range itemIDs {
				for _, labelID := range labelIDs {
					if _, err := tx.Exec(ctx, `
						INSERT INTO omnivore.entity_labels (library_item_id, label_id)
						VALUES ($1, $2) ON CONFLICT DO NOTHING
					`, itemID, labelID); err != nil {
						log.Printf("[bulk-action] add label %s to item %s: %v", labelID, itemID, err)
					}
				}
			}
			count = len(itemIDs)
			return nil
		}); err != nil {
			return fmt.Errorf("bulk-add-labels: %w", err)
		}
		processed += count
		if count < limit {
			break
		}
	}
	log.Printf("[bulk-action] added labels to %d items for user=%s", processed, userID)
	return nil
}

func parseFolderArg(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("args is empty")
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	folder, ok := m["folder"]
	if !ok || folder == "" {
		return "", fmt.Errorf("folder not found in args")
	}
	return folder, nil
}
