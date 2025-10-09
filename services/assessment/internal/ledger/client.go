// v1
// internal/ledger/client.go
package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	circuitbreaker "github.com/nrg-champ/circuitbreaker"
	"github.com/your-org/assessment/internal/logging"
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
	base         string
	h            *http.Client
	breaker      *circuitbreaker.Breaker
	log          *slog.Logger
	rand         *rand.Rand
	randMu       sync.Mutex
	backoffBase  time.Duration
	jitterMax    time.Duration
	resetTimeout time.Duration
}

// ErrCircuitBreakerOpen signals that the Ledger circuit breaker is open and the
// call has been short-circuited. Handlers convert this into a proper upstream
// error response.
var ErrCircuitBreakerOpen = errors.New("ledger circuit breaker open")

// New constructs a Ledger client protected by the shared circuit breaker.
// The breaker keeps the existing 10s request timeout while adding retries and
// health probes so that repeated ledger outages fast-fail.
func New(base string) *Client {
	trimmedBase := strings.TrimRight(base, "/")
	if trimmedBase == "" {
		trimmedBase = base
	}
	h := &http.Client{Timeout: 10 * time.Second}

	logger := newLedgerLogger()
	cbLogFile := os.Getenv("ASSESSMENT_LEDGER_CB_LOGFILE")
	if cbLogFile == "" {
		cbLogFile = os.Getenv("ASSESSMENT_LOGFILE")
	}
	resetTimeout := time.Second
	cbCfg := circuitbreaker.Config{
		MaxFailures:      3,
		ResetTimeout:     resetTimeout,
		SuccessesToClose: 2,
		LogFile:          cbLogFile,
	}
	probeURL := trimmedBase + "/health"
	probe := func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
		if err != nil {
			return err
		}
		resp, err := h.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
			return fmt.Errorf("probe status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		io.CopyN(io.Discard, resp.Body, 64)
		return nil
	}
	breaker := circuitbreaker.New("assessment-ledger-http", cbCfg, probe)
	breaker.SetStateChangeHook(func(state circuitbreaker.State) {
		switch state {
		case circuitbreaker.Open:
			logger.Warn("ledger breaker opened", "base", trimmedBase)
		case circuitbreaker.HalfOpen:
			logger.Info("ledger breaker half-open", "base", trimmedBase)
		case circuitbreaker.Closed:
			logger.Info("ledger breaker closed", "base", trimmedBase)
		}
	})
	logger.Info("ledger breaker initialized", "base", trimmedBase)

	return &Client{
		base:         trimmedBase,
		h:            h,
		breaker:      breaker,
		log:          logger,
		rand:         rand.New(rand.NewSource(time.Now().UnixNano())),
		backoffBase:  200 * time.Millisecond,
		jitterMax:    120 * time.Millisecond,
		resetTimeout: resetTimeout,
	}
}

// newLedgerLogger builds a slog logger that mirrors service logging.
func newLedgerLogger() *slog.Logger {
	dl, err := logging.New()
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return dl.Logger.With("component", "ledger-client")
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

		decoded, err := c.fetchPage(ctx, u.String())
		if err != nil {
			return nil, err
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

type transientHTTPError struct {
	status int
	body   string
}

func (t *transientHTTPError) Error() string {
	return fmt.Sprintf("transient ledger error: status=%d body=%s", t.status, t.body)
}

func (c *Client) fetchPage(ctx context.Context, targetURL string) (eventsPage, error) {
	var page eventsPage
	for attempt := 0; attempt <= 2; attempt++ {
		err := c.breaker.Execute(ctx, func(execCtx context.Context) error {
			req, err := http.NewRequestWithContext(execCtx, http.MethodGet, targetURL, nil)
			if err != nil {
				return err
			}
			resp, err := c.h.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 500 && resp.StatusCode < 600 {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
				return &transientHTTPError{status: resp.StatusCode, body: strings.TrimSpace(string(body))}
			}
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				return fmt.Errorf("ledger %s returned %d: %s", targetURL, resp.StatusCode, strings.TrimSpace(string(body)))
			}
			page = eventsPage{}
			return json.NewDecoder(resp.Body).Decode(&page)
		})
		if err == nil {
			return page, nil
		}
		if errors.Is(err, circuitbreaker.ErrOpen) {
			return eventsPage{}, ErrCircuitBreakerOpen
		}
		var transient *transientHTTPError
		if errors.As(err, &transient) {
			if attempt == 2 {
				return eventsPage{}, fmt.Errorf("ledger transient failure after retries: %w", transient)
			}
			if waitErr := c.waitBackoff(ctx, attempt); waitErr != nil {
				return eventsPage{}, waitErr
			}
			continue
		}
		return eventsPage{}, err
	}
	return eventsPage{}, errors.New("exhausted retries")
}

func (c *Client) waitBackoff(ctx context.Context, attempt int) error {
	base := c.backoffBase * time.Duration(attempt+1)
	jitter := c.jitter()
	delay := base + jitter
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) jitter() time.Duration {
	if c.jitterMax <= 0 || c.rand == nil {
		return 0
	}
	c.randMu.Lock()
	defer c.randMu.Unlock()
	return time.Duration(c.rand.Int63n(int64(c.jitterMax)))
}
