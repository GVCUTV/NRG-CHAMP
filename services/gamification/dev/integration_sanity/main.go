// v0
// services/gamification/dev/integration_sanity/main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

type leaderboardResponse struct {
	Scope   string             `json:"scope"`
	Window  string             `json:"window"`
	Entries []leaderboardEntry `json:"entries"`
}

type leaderboardEntry struct {
	ZoneID    string  `json:"zoneId"`
	EnergyKWh float64 `json:"energyKWh"`
}

type epochPayload struct {
	Type          string      `json:"type"`
	SchemaVersion string      `json:"schemaVersion"`
	ZoneID        string      `json:"zoneId"`
	EpochIndex    int         `json:"epochIndex"`
	MatchedAt     string      `json:"matchedAt"`
	Aggregator    aggEnvelope `json:"aggregator"`
	EnergyTotal   float64     `json:"energyKWh_total"`
	Epoch         string      `json:"epoch"`
}

type aggEnvelope struct {
	Summary aggSummary `json:"summary"`
}

type aggSummary struct {
	ZoneEnergy float64 `json:"zoneEnergyKWhEpoch"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger, file, err := buildLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = file.Close()
	}()

	if err := run(ctx, logger); err != nil {
		logger.Error("integration_sanity_failed", slog.Any("err", err))
		os.Exit(1)
	}
	logger.Info("integration_sanity_complete")
}

func run(ctx context.Context, logger *slog.Logger) error {
	if _, err := os.Stat("docker-compose.yml"); err != nil {
		return fmt.Errorf("docker-compose.yml not found: %w", err)
	}

	if err := ensureServices(ctx, logger); err != nil {
		return err
	}

	if err := waitForHTTP(ctx, logger, "http://localhost:8085/health/ready", 2*time.Minute, 5*time.Second); err != nil {
		return fmt.Errorf("gamification readiness: %w", err)
	}

	payload, err := buildPayload()
	if err != nil {
		return err
	}

	zoneID := "zone-001"
	logSince := time.Now().UTC()

	if err := publishSample(ctx, logger, zoneID, payload); err != nil {
		return err
	}

	if err := waitForLeaderboardEntry(ctx, logger, zoneID, payload.Aggregator.Summary.ZoneEnergy, 2*time.Minute); err != nil {
		return err
	}

	if err := ensureNoEpochWarnings(ctx, logger, logSince, "gamification"); err != nil {
		return err
	}

	if err := ensureNoEpochWarnings(ctx, logger, logSince, "ledger"); err != nil {
		return err
	}

	return nil
}

func ensureServices(ctx context.Context, logger *slog.Logger) error {
	logger.Info("starting_services")
	cmd := exec.CommandContext(ctx, "docker", "compose", "up", "-d", "kafka", "topic-init", "ledger", "gamification")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	logger.Info("services_started")
	return nil
}

func waitForHTTP(ctx context.Context, logger *slog.Logger, url string, timeout, interval time.Duration) error {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return errors.New("timeout waiting for http endpoint")
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			logger.Info("http_endpoint_ready", slog.String("url", url))
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}

		logger.Info("http_endpoint_wait", slog.String("url", url))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func buildPayload() (epochPayload, error) {
	now := time.Now().UTC()
	payload := epochPayload{
		Type:          "epoch.public",
		SchemaVersion: "v1",
		ZoneID:        "zone-001",
		EpochIndex:    7,
		MatchedAt:     now.Format(time.RFC3339Nano),
		Aggregator:    aggEnvelope{Summary: aggSummary{ZoneEnergy: 42.5}},
		EnergyTotal:   99.1,
		Epoch:         now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
	}
	return payload, nil
}

func publishSample(ctx context.Context, logger *slog.Logger, zoneID string, payload epochPayload) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return fmt.Errorf("compact payload: %w", err)
	}

	script := fmt.Sprintf("docker compose exec -T kafka kafka-console-producer.sh --bootstrap-server kafka:9092 --topic ledger.public.epochs --property parse.key=true --property key.separator='::' <<'EOF'\n%s::%s\nEOF", zoneID, buf.String())

	logger.Info("publishing_sample", slog.String("zoneId", zoneID))
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("publish sample: %w", err)
	}
	logger.Info("sample_published", slog.String("zoneId", zoneID))
	return nil
}

func waitForLeaderboardEntry(ctx context.Context, logger *slog.Logger, zoneID string, expectedEnergy float64, timeout time.Duration) error {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for leaderboard entry for %s", zoneID)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8085/leaderboard?window=24h", nil)
		if err != nil {
			return fmt.Errorf("build leaderboard request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			logger.Warn("leaderboard_request_failed", slog.Any("err", err))
		} else {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				logger.Warn("leaderboard_read_failed", slog.Any("err", readErr))
			} else if resp.StatusCode == http.StatusOK {
				var payload leaderboardResponse
				if err := json.Unmarshal(body, &payload); err != nil {
					logger.Warn("leaderboard_decode_failed", slog.Any("err", err))
				} else {
					if strings.ToLower(payload.Scope) != "global" {
						return fmt.Errorf("unexpected scope %q", payload.Scope)
					}
					if strings.ToLower(payload.Window) != "24h" {
						return fmt.Errorf("unexpected window %q", payload.Window)
					}
					for _, entry := range payload.Entries {
						if strings.EqualFold(entry.ZoneID, zoneID) {
							if math.Abs(entry.EnergyKWh-expectedEnergy) <= 0.0001 {
								logger.Info("leaderboard_entry_verified", slog.String("zoneId", entry.ZoneID), slog.Float64("energyKWh", entry.EnergyKWh))
								return nil
							}
							return fmt.Errorf("zone %s energy mismatch: got %.4f want %.4f", entry.ZoneID, entry.EnergyKWh, expectedEnergy)
						}
					}
					logger.Info("leaderboard_entry_pending", slog.Int("entries", len(payload.Entries)))
				}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func ensureNoEpochWarnings(ctx context.Context, logger *slog.Logger, since time.Time, service string) error {
	sinceValue := since.Format(time.RFC3339)
	logger.Info("checking_logs", slog.String("service", service), slog.String("since", sinceValue))
	cmd := exec.CommandContext(ctx, "docker", "compose", "logs", "--no-color", "--since", sinceValue, service)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose logs %s: %w", service, err)
	}

	if bytes.Contains(bytes.ToLower(output), []byte("epoch field missing")) {
		return fmt.Errorf("found 'epoch field missing' in %s logs", service)
	}
	logger.Info("logs_clean", slog.String("service", service))
	return nil
}

func buildLogger() (*slog.Logger, *os.File, error) {
	path := filepath.Join("logs", "dev", "gamification-integration.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create log directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	handler := slog.NewJSONHandler(io.MultiWriter(os.Stdout, file), &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)
	logger.Info("logger_initialized", slog.String("log_path", path))
	return logger, file, nil
}
