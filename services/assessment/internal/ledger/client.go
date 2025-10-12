// v1
// internal/ledger/client.go
package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Event represents a generic ledger event we expect from the Ledger service.
// We keep the structure permissive to tolerate upstream changes.
// Expected types include: "reading", "action", "anomaly" (but this service only *reads*, never writes).
type Event struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"` // reading | action | anomaly | ...
	ZoneID  string         `json:"zoneId"`
	Ts      time.Time      `json:"ts"`
	Payload map[string]any `json:"payload"` // flexible
}

type paginatedResponse struct {
	Total int     `json:"total"`
	Page  int     `json:"page"`
	Size  int     `json:"size"`
	Items []Event `json:"items"`
}

type Client struct {
	base string
	h    *http.Client
}

func New(base string) *Client {
	return &Client{
		base: base,
		h:    &http.Client{Timeout: 10 * time.Second},
	}
}

// FetchEvents calls the Ledger: GET /events?type=&zoneId=&from=&to=&page=&size=
// and returns the aggregated list for the requested window. Pagination is followed until exhaustion.
func (c *Client) FetchEvents(ctx context.Context, typ, zoneID string, from, to time.Time) ([]Event, error) {
	var out []Event
	page := 1
	size := 500

	for {
		u, err := url.Parse(c.base + "/events")
		if err != nil {
			return nil, err
		}
		q := u.Query()
		if typ != "" {
			q.Set("type", typ)
		}
		if zoneID != "" {
			q.Set("zoneId", zoneID)
		}
		if !from.IsZero() {
			q.Set("from", from.Format(time.RFC3339))
		}
		if !to.IsZero() {
			q.Set("to", to.Format(time.RFC3339))
		}
		q.Set("page", fmt.Sprintf("%d", page))
		q.Set("size", fmt.Sprintf("%d", size))
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.h.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("ledger %s returned %d: %s", u.String(), resp.StatusCode, string(b))
		}
		var payload paginatedResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		if payload.Items == nil {
			return nil, fmt.Errorf("ledger %s returned response without items", u.String())
		}
		out = append(out, payload.Items...)

		effectiveSize := size
		if payload.Size > 0 {
			effectiveSize = payload.Size
		}

		if len(payload.Items) == 0 {
			break
		}
		if payload.Total > 0 && len(out) >= payload.Total {
			break
		}
		if len(payload.Items) < effectiveSize {
			break
		}

		if payload.Page > 0 {
			page = payload.Page + 1
		} else {
			page++
		}

		if effectiveSize > 0 {
			size = effectiveSize
		}

		if size <= 0 {
			break
		}
		if page <= 0 {
			break
		}
	}
	return out, nil
}
