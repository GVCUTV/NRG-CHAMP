// v0
// internal/core/ledger_client.go
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// LedgerClient fetches events strictly via HTTP from the Ledger service.
type LedgerClient struct {
	cfg  Config
	lg   *Logger
	h    *http.Client
	base string
}

// NewLedgerClient creates a new client with sane timeouts.
func NewLedgerClient(cfg Config, lg *Logger) *LedgerClient {
	return &LedgerClient{
		cfg:  cfg,
		lg:   lg,
		h:    &http.Client{Timeout: 20 * time.Second},
		base: cfg.LedgerBaseURL,
	}
}

// FetchEvents retrieves events for a zone and window. If 'etype' is non-empty, it filters by type.
func (c *LedgerClient) FetchEvents(ctx context.Context, zoneId string, from, to time.Time, etype string) ([]Event, error) {
	q := url.Values{}
	q.Set("zoneId", zoneId)
	q.Set("from", from.UTC().Format(time.RFC3339))
	q.Set("to", to.UTC().Format(time.RFC3339))
	if etype != "" {
		q.Set("type", etype)
	}
	// We attempt pagination; if the ledger supports page/size, we iterate; otherwise, we try a single large size.
	size := 1000
	page := 0
	var all []Event

	for {
		params := url.Values{}
		for k, v := range q {
			params[k] = v
		}
		params.Set("page", fmt.Sprintf("%d", page))
		params.Set("size", fmt.Sprintf("%d", size))

		endpoint := fmt.Sprintf("%s/events?%s", c.base, params.Encode())
		c.lg.Info("ledger fetch", "endpoint", endpoint)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.h.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("ledger http %d: %s", resp.StatusCode, string(b))
		}

		// We accept either an envelope {{items:[]}} or a bare array.
		var arr []Event
		dec := json.NewDecoder(resp.Body)
		// Peek the first token to detect object vs array
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t {
		case json.Delim('['):
			// Bare array
			var e Event
			for dec.More() {
				if err := dec.Decode(&e); err != nil {
					return nil, err
				}
				arr = append(arr, e)
			}
			// consume closing ]
			_, _ = dec.Token()
		case json.Delim('{'):
			// Envelope: we expect a key "items" carrying the array
			var raw map[string]json.RawMessage
			raw = make(map[string]json.RawMessage)
			// read the rest of the object
			if err := dec.Decode(&raw); err != nil {
				return nil, err
			}
			if v, ok := raw["items"]; ok {
				if err := json.Unmarshal(v, &arr); err != nil {
					return nil, err
				}
			} else {
				// fallback: try to unmarshal the entire object as an array field named "data"
				if v, ok := raw["data"]; ok {
					if err := json.Unmarshal(v, &arr); err != nil {
						return nil, err
					}
				}
			}
		default:
			return nil, fmt.Errorf("unexpected JSON from ledger")
		}

		all = append(all, arr...)

		// Stop if we got fewer than 'size' — naive pagination stop condition.
		if len(arr) < size {
			break
		}
		page++
		// small safety cap
		if page > 1000 {
			break
		}
	}

	return all, nil
}
