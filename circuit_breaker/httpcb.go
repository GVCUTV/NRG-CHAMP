// v0
// httpcb.go
package circuit_breaker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient wraps a standard http.Client with circuit breaker behavior.
type HTTPClient struct {
	Client   *http.Client
	brk      *Breaker
	probeURL string
}

func NewHTTPClient(name string, cfg Config, probeURL string, httpClient *http.Client) (*HTTPClient, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	probe := func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
		if err != nil {
			return err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		io.CopyN(io.Discard, resp.Body, 64)
		if resp.StatusCode >= 200 && resp.StatusCode < 500 {
			return nil
		}
		return fmt.Errorf("probe_bad_status: %d", resp.StatusCode)
	}
	brk := New(name, cfg, probe)
	return &HTTPClient{Client: httpClient, brk: brk, probeURL: probeURL}, nil
}

func (h *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	err := h.brk.Execute(req.Context(), func(ctx context.Context) error {
		req = req.WithContext(ctx)
		r, err := h.Client.Do(req)
		if err != nil {
			return err
		}
		resp = r
		return nil
	})
	return resp, err
}
