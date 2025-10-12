// v1
// internal/ledger/client.go
package ledger

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	circuitbreaker "github.com/nrg-champ/circuitbreaker"
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

type Client struct {
	base        string
	http        *circuitbreaker.HTTPClient
	log         *slog.Logger
	retryMin    time.Duration
	retryMax    time.Duration
	maxAttempts int
}

type BreakerSettings struct {
	FailureThreshold int
	ResetTimeout     time.Duration
	HalfOpenMax      int
	LogFile          string
}

func New(base string, logger *slog.Logger, cfg BreakerSettings) (*Client, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 30 * time.Second
	}
	if cfg.HalfOpenMax <= 0 {
		cfg.HalfOpenMax = 1
	}
	trimmed := strings.TrimSuffix(base, "/")
	httpClient := &http.Client{Timeout: 10 * time.Second}
	breakerCfg := circuitbreaker.Config{
		MaxFailures:      cfg.FailureThreshold,
		ResetTimeout:     cfg.ResetTimeout,
		SuccessesToClose: cfg.HalfOpenMax,
		LogFile:          cfg.LogFile,
	}
	probeURL := trimmed + "/health"
	cb, err := circuitbreaker.NewHTTPClient("assessment-ledger", breakerCfg, probeURL, httpClient)
	if err != nil {
		return nil, err
	}
	logger.Info("ledger_client_initialized", "base", trimmed, "cbFailures", breakerCfg.MaxFailures, "cbReset", breakerCfg.ResetTimeout, "cbHalfOpen", breakerCfg.SuccessesToClose)
	return &Client{
		base:        trimmed,
		http:        cb,
		log:         logger,
		retryMin:    50 * time.Millisecond,
		retryMax:    150 * time.Millisecond,
		maxAttempts: 2,
	}, nil
}

// FetchEvents calls the Ledger: GET /events?type=&zoneId=&from=&to=&page=&size=
// and returns the aggregated list for the requested window. Pagination is followed until exhaustion.
type eventsPage struct {
	Total int     `json:"total"`
	Page  int     `json:"page"`
	Size  int     `json:"size"`
	Items []Event `json:"items"`
}

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

		pageURL := u.String()
		c.log.Info("ledger_fetch_page_start", "url", pageURL, "page", page, "size", size)

		var decoded eventsPage
		var success bool

		for attempt := 1; attempt <= c.maxAttempts; attempt++ {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
			if err != nil {
				return nil, err
			}

			resp, err := c.http.Do(req)
			if err != nil {
				if errors.Is(err, circuitbreaker.ErrOpen) {
					if resp != nil && resp.Body != nil {
						resp.Body.Close()
					}
					c.log.Error("ledger_circuit_open", "url", pageURL)
					return nil, err
				}
				if resp != nil {
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode >= http.StatusInternalServerError && attempt < c.maxAttempts {
						c.log.Warn("ledger_fetch_retry_after_5xx", "url", pageURL, "status", resp.StatusCode, "attempt", attempt)
						if err := c.sleepWithJitter(ctx); err != nil {
							return nil, err
						}
						continue
					}
					if resp.StatusCode >= http.StatusInternalServerError {
						return nil, fmt.Errorf("ledger %s returned %d: %s", pageURL, resp.StatusCode, string(body))
					}
					return nil, fmt.Errorf("ledger %s returned %d: %s", pageURL, resp.StatusCode, string(body))
				}
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, err
			}

			if resp.StatusCode >= http.StatusInternalServerError {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if attempt < c.maxAttempts {
					c.log.Warn("ledger_fetch_retry_after_5xx", "url", pageURL, "status", resp.StatusCode, "attempt", attempt)
					if err := c.sleepWithJitter(ctx); err != nil {
						return nil, err
					}
					continue
				}
				return nil, fmt.Errorf("ledger %s returned %d: %s", pageURL, resp.StatusCode, string(body))
			}

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				return nil, fmt.Errorf("ledger %s returned %d: %s", pageURL, resp.StatusCode, string(body))
			}

			if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
				resp.Body.Close()
				return nil, err
			}
			resp.Body.Close()
			c.log.Info("ledger_fetch_page_success", "url", pageURL, "page", page, "attempt", attempt, "items", len(decoded.Items))
			success = true
			break
		}

		if !success {
			return nil, fmt.Errorf("ledger %s exhausted retries", pageURL)
		}
		if decoded.Size > 0 {
			size = decoded.Size
		}
		if len(decoded.Items) == 0 {
			break
		}
		out = append(out, decoded.Items...)
		if decoded.Total > 0 {
			if len(out) >= decoded.Total {
				break
			}
			if decoded.Page > 0 && decoded.Size > 0 {
				consumed := decoded.Page * decoded.Size
				if consumed >= decoded.Total {
					break
				}
			}
		} else if len(decoded.Items) < size {
			break
		}
		page++
	}
	return out, nil
}

func (c *Client) sleepWithJitter(ctx context.Context) error {
	delay := c.jitterDuration(c.retryMin, c.retryMax)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) jitterDuration(min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	delta := max - min
	n, err := rand.Int(rand.Reader, big.NewInt(int64(delta)))
	if err != nil {
		return min
	}
	return min + time.Duration(n.Int64())
}
