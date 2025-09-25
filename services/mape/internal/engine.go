// v2
// engine.go
package internal

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"
)

type Engine struct {
	cfg   *AppConfig
	lg    *slog.Logger
	io    *IO
	stats Stats
}
type Stats struct{ Loops, MessagesIn, CommandsOut, LedgerWrites int64 }

func NewEngine(cfg *AppConfig, lg *slog.Logger, io *IO) *Engine {
	return &Engine{cfg: cfg, lg: lg, io: io}
}

func (e *Engine) Run(ctx context.Context) {
	interval := time.Duration(e.cfg.PollIntervalMs) * time.Millisecond
	e.lg.Info("engine start", "interval_ms", e.cfg.PollIntervalMs, "zones", e.cfg.Zones)
	for {
		select {
		case <-ctx.Done():
			e.lg.Info("engine stop")
			return
		default:
		}
		for _, zone := range e.cfg.Zones {
			latest, epoch, ok, err := e.io.DrainZonePartitionLatest(ctx, zone)
			if err != nil {
				e.lg.Error("drain", "zone", zone, "err", err)
				continue
			}
			if !ok {
				continue
			}
			e.stats.MessagesIn++
			cmds, led := e.planForZone(zone, latest, epoch)
			if err := e.io.PublishCommandsAndLedger(ctx, zone, cmds, led); err != nil {
				e.lg.Error("publish", "zone", zone, "err", err)
				continue
			}
			e.stats.CommandsOut += int64(len(cmds))
			e.stats.LedgerWrites++
		}
		e.stats.Loops++
		time.Sleep(interval)
	}
}

func (e *Engine) planForZone(zone string, latest Reading, epoch int64) ([]PlanCommand, LedgerEvent) {
	t := e.cfg.ZoneTargets[zone]
	h := e.cfg.ZoneHysteresis[zone]
	var temp *float64
	if m, ok := latest.Reading.(map[string]any); ok {
		if v, ok := m["tempC"]; ok {
			switch x := v.(type) {
			case float64:
				temp = &x
			}
		}
	}
	mode := "OFF"
	fan := 0
	reason := "within hysteresis"
	delta := 0.0
	if temp != nil {
		delta = *temp - t
		if delta > h {
			mode = "COOL"
			fan = e.pickFan(math.Abs(delta))
			reason = fmt.Sprintf("too hot by %.2fC", delta)
		} else if delta < -h {
			mode = "HEAT"
			fan = e.pickFan(math.Abs(delta))
			reason = fmt.Sprintf("too cold by %.2fC", -delta)
		}
	}
	cmds := []PlanCommand{
		{ZoneID: zone, ActuatorID: "heating.1", Mode: ifMode(mode, "HEAT"), FanPercent: ifFan(mode, "HEAT", fan), Reason: reason, EpochMs: epoch, IssuedAt: time.Now().UnixMilli()},
		{ZoneID: zone, ActuatorID: "cooling.1", Mode: ifMode(mode, "COOL"), FanPercent: ifFan(mode, "COOL", fan), Reason: reason, EpochMs: epoch, IssuedAt: time.Now().UnixMilli()},
	}
	led := LedgerEvent{EpochMs: epoch, ZoneID: zone, Planned: fmt.Sprintf("mode=%s fan=%d reason=%s", mode, fan, reason), TargetC: t, HystC: h, DeltaC: delta, Fan: fan, Timestamp: time.Now().UnixMilli()}
	return cmds, led
}
func (e *Engine) pickFan(x float64) int {
	for i, s := range e.cfg.FanSteps {
		if x <= s {
			return e.cfg.FanSpeeds[i]
		}
	}
	if n := len(e.cfg.FanSpeeds); n > 0 {
		return e.cfg.FanSpeeds[n-1]
	}
	return 0
}
func ifMode(sel, want string) string {
	if sel == want {
		return want
	}
	return "OFF"
}
func ifFan(sel, want string, fan int) int {
	if sel == want {
		return fan
	}
	return 0
}
func (e *Engine) Stats() Stats { return e.stats }
