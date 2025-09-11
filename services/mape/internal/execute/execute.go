// v1
// internal/execute/execute.go
package execute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"nrg-champ/mape/full/internal/config"
	"nrg-champ/mape/full/internal/kafkabus"
	"nrg-champ/mape/full/internal/plan"
)

type Executor struct {
	cfg config.Config
	log *slog.Logger
	bus *kafkabus.Bus
	hc  *http.Client
}

func New(cfg config.Config, log *slog.Logger, bus *kafkabus.Bus) *Executor {
	return &Executor{cfg: cfg, log: log.With(slog.String("component", "executor")), bus: bus, hc: &http.Client{Timeout: 5 * time.Second}}
}

func (e *Executor) Execute(ctx context.Context, room string, cmd *plan.Command) error {
	if cmd == nil {
		return nil
	}
	switch e.cfg.ExecuteMode {
	case "http":
		return e.execHTTP(ctx, room, cmd)
	case "kafka":
		return e.execKafka(ctx, room, cmd)
	default:
		return e.execHTTP(ctx, room, cmd)
	}
}

func (e *Executor) execHTTP(ctx context.Context, room string, cmd *plan.Command) error {
	base := strings.ReplaceAll(e.cfg.RoomHTTPBase, "{room}", room)
	// endpoints: /actuators/heating, /actuators/cooling, /actuators/ventilation
	e.log.Info("exec http", slog.String("room", room), slog.Any("cmd", cmd))

	if err := e.postJSON(ctx, base+"/actuators/heating", map[string]any{"state": tern(cmd.HeaterOn, "ON", "OFF")}); err != nil {
		return err
	}
	if err := e.postJSON(ctx, base+"/actuators/cooling", map[string]any{"state": tern(cmd.CoolerOn, "ON", "OFF")}); err != nil {
		return err
	}
	if err := e.postJSON(ctx, base+"/actuators/ventilation", map[string]any{"state": fmt.Sprintf("%d", cmd.FanPct)}); err != nil {
		return err
	}
	return nil
}

func (e *Executor) execKafka(ctx context.Context, room string, cmd *plan.Command) error {
	topic := e.cfg.CommandTopicPrefix + "." + room
	w := e.bus.Writer(topic)
	msg := map[string]any{
		"roomId":    room,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"command":   cmd,
	}
	b, _ := json.Marshal(msg)
	e.log.Info("exec kafka", slog.String("topic", topic), slog.Any("cmd", cmd))
	return w.WriteMessages(ctx, kafka.Message{Value: b})
}

func (e *Executor) postJSON(ctx context.Context, url string, body any) error {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("non-2xx from %s: %d", url, resp.StatusCode)
	}
	return nil
}

func tern[T any](c bool, a, b T) T {
	if c {
		return a
	}
	return b
}
