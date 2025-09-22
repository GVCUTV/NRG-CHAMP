// v0
// engine.go
package mape

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"nrgchamp/mape/internal/config"
	"nrgchamp/mape/internal/kafkaio"
)

// Engine coordinates the MAPE control loop.
type Engine struct {
	cfg   *config.AppConfig
	lg    *slog.Logger
	io    *kafkaio.IO
	stats Stats
}

// Stats holds some counters for /status endpoint.
type Stats struct {
	Loops        int64 `json:"loops"`
	MessagesIn   int64 `json:"messagesIn"`
	CommandsOut  int64 `json:"commandsOut"`
	LedgerWrites int64 `json:"ledgerWrites"`
}

func NewEngine(cfg *config.AppConfig, lg *slog.Logger, io *kafkaio.IO) *Engine {
	return &Engine{cfg: cfg, lg: lg, io: io}
}

// Run executes the round-robin loop over all partitions (zones) repeatedly.
func (e *Engine) Run(ctx context.Context) {
	interval := time.Duration(e.cfg.PollIntervalMs) * time.Millisecond
	e.lg.Info("engine loop starting", "interval_ms", e.cfg.PollIntervalMs, "zones", e.cfg.Zones)

	for {
		select {
		case <-ctx.Done():
			e.lg.Info("engine loop exiting")
			return
		default:
			// Round robin zones/partitions
			for _, zone := range e.cfg.Zones {
				if ctx.Err() != nil {
					break
				}
				latest, epochMs, ok, err := e.io.DrainZonePartitionLatest(ctx, zone)
				if err != nil {
					e.lg.Error("drain error", "zone", zone, "error", err)
					continue
				}
				if !ok {
					continue // no messages in this epoch for this zone
				}
				e.stats.MessagesIn++
				// Analyze + Plan
				cmds, ledger := e.planForZone(zone, latest, epochMs)
				// Execute
				if err := e.io.PublishCommandsAndLedger(ctx, zone, cmds, ledger); err != nil {
					e.lg.Error("publish error", "zone", zone, "error", err)
					continue
				}
				e.stats.CommandsOut += int64(len(cmds))
				e.stats.LedgerWrites++
			}
			e.stats.Loops++
			time.Sleep(interval)
		}
	}
}

// planForZone builds commands from latest reading vs targets & hysteresis.
func (e *Engine) planForZone(zone string, latest Reading, epochMs int64) ([]PlanCommand, LedgerEvent) {
	target := e.cfg.ZoneTargets[zone]
	hys := e.cfg.ZoneHysteresis[zone]
	var temp *float64

	switch r := latest.Reading.(type) {
	case map[string]any:
		if v, ok := r["tempC"]; ok {
			if f, ok2 := toFloat64(v); ok2 {
				temp = &f
			}
		}
	default:
		// try JSON re-marshal if different type sneaks in
		b, _ := json.Marshal(latest.Reading)
		var x map[string]any
		_ = json.Unmarshal(b, &x)
		if v, ok := x["tempC"]; ok {
			if f, ok2 := toFloat64(v); ok2 {
				temp = &f
			}
		}
	}

	mode := "OFF"
	fan := 0
	reason := "within hysteresis"
	delta := 0.0
	if temp != nil {
		delta = *temp - target
		if delta > hys { // too hot -> COOL
			mode = "COOL"
			reason = fmt.Sprintf("too hot by %.2fC (> hysteresis %.2fC)", delta, hys)
			fan = e.pickFan(math.Abs(delta))
		} else if delta < -hys { // too cold -> HEAT
			mode = "HEAT"
			reason = fmt.Sprintf("too cold by %.2fC (> hysteresis %.2fC)", -delta, hys)
			fan = e.pickFan(math.Abs(delta))
		} else {
			mode = "OFF"
			fan = 0
		}
	} else {
		reason = "no temperature in latest reading; staying OFF"
	}

	// For now, we produce one command for heating and one for cooling actuator.
	cmds := []PlanCommand{
		{ZoneID: zone, ActuatorID: "heating.1", Mode: modeIf(mode, "HEAT"), FanPercent: fanIf(mode, "HEAT", fan), Reason: reason, EpochMs: epochMs, IssuedAt: time.Now().UnixMilli()},
		{ZoneID: zone, ActuatorID: "cooling.1", Mode: modeIf(mode, "COOL"), FanPercent: fanIf(mode, "COOL", fan), Reason: reason, EpochMs: epochMs, IssuedAt: time.Now().UnixMilli()},
	}
	led := LedgerEvent{
		EpochMs:   epochMs,
		ZoneID:    zone,
		Planned:   fmt.Sprintf("mode=%s fan=%d reason=%s", mode, fan, reason),
		TargetC:   target,
		HystC:     hys,
		DeltaC:    delta,
		Fan:       fan,
		Timestamp: time.Now().UnixMilli(),
	}
	return cmds, led
}

func (e *Engine) pickFan(absDelta float64) int {
	for i, step := range e.cfg.FanSteps {
		if absDelta <= step {
			return e.cfg.FanSpeeds[i]
		}
	}
	// if larger than all steps, return the last (max) speed
	if n := len(e.cfg.FanSpeeds); n > 0 {
		return e.cfg.FanSpeeds[n-1]
	}
	return 0
}

func toFloat64(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func modeIf(selected, want string) string {
	if selected == want {
		return want
	}
	return "OFF"
}
func fanIf(selected, want string, fan int) int {
	if selected == want {
		return fan
	}
	return 0
}

// Exposed for /status endpoint.
func (e *Engine) Stats() Stats { return e.stats }
