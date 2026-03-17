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

// TriggerRuleData is the job payload for trigger-rule.
type TriggerRuleData struct {
	UserID        string          `json:"userId"`
	RuleEventType string          `json:"ruleEventType"` // e.g. "PAGE_CREATED"
	Data          json.RawMessage `json:"data"`          // library item fields as a map
}

type ruleAction struct {
	Type   string   `json:"type"`
	Params []string `json:"params"`
}

type ruleRow struct {
	ID      string
	Name    string
	Actions []ruleAction
}

// HandleTriggerRule evaluates and applies matching rules for a user event.
func HandleTriggerRule(ctx context.Context, cfg *config.Config, redisDS *redisutil.RedisDataSource, dbPool *db.Pool, data []byte) error {
	var d TriggerRuleData
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("trigger-rule: unmarshal: %w", err)
	}

	// Extract library item id from the event data.
	var itemData map[string]any
	if err := json.Unmarshal(d.Data, &itemData); err != nil {
		return fmt.Errorf("trigger-rule: unmarshal item data: %w", err)
	}
	libraryItemID, _ := itemData["id"].(string)
	if libraryItemID == "" {
		log.Printf("[trigger-rule] no library item id in event data, skipping")
		return nil
	}

	// Query matching enabled rules.
	rows, err := dbPool.Query(ctx, `
		SELECT id, name, actions
		FROM omnivore.rules
		WHERE user_id = $1 AND enabled = true AND $2 = ANY(event_types)
	`, d.UserID, d.RuleEventType)
	if err != nil {
		return fmt.Errorf("trigger-rule: query rules: %w", err)
	}
	defer rows.Close()

	var rules []ruleRow
	for rows.Next() {
		var r ruleRow
		var actionsJSON []byte
		if err := rows.Scan(&r.ID, &r.Name, &actionsJSON); err != nil {
			return fmt.Errorf("trigger-rule: scan rule: %w", err)
		}
		if err := json.Unmarshal(actionsJSON, &r.Actions); err != nil {
			log.Printf("[trigger-rule] rule id=%s: invalid actions JSON: %v", r.ID, err)
			continue
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("trigger-rule: rows: %w", err)
	}

	for _, rule := range rules {
		// Verify item still exists.
		var itemID string
		err := dbPool.QueryRow(ctx, `
			SELECT id FROM omnivore.library_item WHERE id = $1 AND user_id = $2
		`, libraryItemID, d.UserID).Scan(&itemID)
		if err != nil {
			log.Printf("[trigger-rule] rule=%s item=%s not found, skipping", rule.ID, libraryItemID)
			continue
		}

		for _, action := range rule.Actions {
			if err := applyRuleAction(ctx, dbPool, action, libraryItemID, d.UserID); err != nil {
				log.Printf("[trigger-rule] rule=%s action=%s failed: %v", rule.ID, action.Type, err)
			}
		}
		log.Printf("[trigger-rule] applied rule=%s (%s) to item=%s", rule.ID, rule.Name, libraryItemID)
	}
	return nil
}

func applyRuleAction(ctx context.Context, dbPool *db.Pool, action ruleAction, libraryItemID, userID string) error {
	switch action.Type {
	case "ADD_LABEL":
		for _, labelID := range action.Params {
			if _, err := dbPool.Exec(ctx, `
				INSERT INTO omnivore.entity_labels (library_item_id, label_id)
				VALUES ($1, $2) ON CONFLICT DO NOTHING
			`, libraryItemID, labelID); err != nil {
				log.Printf("[trigger-rule] add label %s to item %s: %v", labelID, libraryItemID, err)
			}
		}
	case "ARCHIVE":
		if _, err := dbPool.Exec(ctx, `
			UPDATE omnivore.library_item
			SET state = 'ARCHIVED', archived_at = now(), updated_at = now()
			WHERE id = $1 AND user_id = $2
		`, libraryItemID, userID); err != nil {
			return fmt.Errorf("archive: %w", err)
		}
	case "DELETE":
		if _, err := dbPool.Exec(ctx, `
			UPDATE omnivore.library_item
			SET state = 'DELETED', deleted_at = now(), updated_at = now()
			WHERE id = $1 AND user_id = $2
		`, libraryItemID, userID); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
	case "MARK_AS_READ":
		if _, err := dbPool.Exec(ctx, `
			UPDATE omnivore.library_item
			SET reading_progress_bottom_percent = 100,
			    reading_progress_top_percent = 100,
			    read_at = now()
			WHERE id = $1 AND user_id = $2
		`, libraryItemID, userID); err != nil {
			return fmt.Errorf("mark-as-read: %w", err)
		}
	case "WEBHOOK":
		if len(action.Params) > 0 {
			webhookURL := action.Params[0]
			payload := map[string]any{
				"action": "rule_triggered",
				"userId": userID,
				"itemId": libraryItemID,
			}
			if err := callWebhookURL(ctx, webhookURL, "", "", payload); err != nil {
				return fmt.Errorf("webhook: %w", err)
			}
		}
	case "SEND_NOTIFICATION", "EXPORT":
		log.Printf("[trigger-rule] action %q not implemented, skipping", action.Type)
	default:
		log.Printf("[trigger-rule] unknown action %q, skipping", action.Type)
	}
	return nil
}
