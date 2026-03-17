package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

// CallWebhookData is the job payload for call-webhook.
type CallWebhookData struct {
	Data   json.RawMessage `json:"data"`
	UserID string          `json:"userId"`
	Type   string          `json:"type"`   // "page"|"label"|"highlight"
	Action string          `json:"action"` // "created"|"updated"|"deleted"
}

type webhookRow struct {
	ID          string
	URL         string
	Method      string
	ContentType string
}

// HandleCallWebhook fires all enabled webhooks matching the event type for a user.
func HandleCallWebhook(ctx context.Context, cfg *config.Config, redisDS *redisutil.RedisDataSource, dbPool *db.Pool, data []byte) error {
	var d CallWebhookData
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("call-webhook: unmarshal: %w", err)
	}

	eventType := strings.ToUpper(d.Type + "_" + d.Action)

	var hooks []webhookRow
	if err := dbPool.AuthTrx(ctx, d.UserID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, url, method, content_type
			FROM omnivore.webhooks
			WHERE user_id = $1 AND enabled = true AND $2 = ANY(event_types)
		`, d.UserID, eventType)
		if err != nil {
			return fmt.Errorf("query webhooks: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var h webhookRow
			if err := rows.Scan(&h.ID, &h.URL, &h.Method, &h.ContentType); err != nil {
				return fmt.Errorf("scan webhook: %w", err)
			}
			hooks = append(hooks, h)
		}
		return rows.Err()
	}); err != nil {
		return fmt.Errorf("call-webhook: query webhooks: %w", err)
	}

	for _, h := range hooks {
		if err := fireWebhook(ctx, h, d); err != nil {
			log.Printf("[call-webhook] webhook id=%s url=%s failed: %v", h.ID, h.URL, err)
		}
	}
	return nil
}

func fireWebhook(ctx context.Context, hook webhookRow, d CallWebhookData) error {
	payload := map[string]any{
		"action":  d.Action,
		"userId":  d.UserID,
		d.Type:    json.RawMessage(d.Data),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	method := hook.Method
	if method == "" {
		method = http.MethodPost
	}
	contentType := hook.ContentType
	if contentType == "" {
		contentType = "application/json"
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, hook.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// queryWebhooksForURL is a helper used by trigger-rule to fire a single webhook URL.
func callWebhookURL(ctx context.Context, webhookURL, method, contentType string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	if method == "" {
		method = http.MethodPost
	}
	if contentType == "" {
		contentType = "application/json"
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}


