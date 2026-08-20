// Package recommendevents posts optional feedback to pf-recommend (append-only log).
package recommendevents

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Line is one purchased catalog row (item_id is SKU).
type Line struct {
	SKU string
	Qty int
}

// PostPurchase fires best-effort purchase events. Empty baseURL is a no-op.
func PostPurchase(ctx context.Context, baseURL, userID string, lines []Line, client *http.Client) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" || strings.TrimSpace(userID) == "" || len(lines) == 0 {
		return
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	for _, ln := range lines {
		sku := strings.TrimSpace(ln.SKU)
		if sku == "" {
			continue
		}
		qty := ln.Qty
		if qty < 1 {
			qty = 1
		}
		for i := 0; i < qty; i++ {
			body, err := json.Marshal(map[string]string{
				"namespace": "commerce",
				"user_id":   userID,
				"item_id":   sku,
				"type":      "purchase",
			})
			if err != nil {
				return
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/events", bytes.NewReader(body))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			res, err := client.Do(req)
			if res != nil {
				_ = res.Body.Close()
			}
			if err != nil {
				return
			}
		}
	}
}
