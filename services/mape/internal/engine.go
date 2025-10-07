// v8
// engine.go
package internal

import (
	"context"
	"log/slog"
	"time"
)

type Engine struct {
	cfg   *AppConfig
	lg    *slog.Logger
	io    *KafkaIO
	mon   *Monitor
	an    *Analyze
	pln   *Plan
	exe   *Execute
	stats Stats
}

var engineRef *Engine

func NewEngine(cfg *AppConfig, lg *slog.Logger, io *KafkaIO) *Engine {
	e := &Engine{cfg: cfg, lg: lg, io: io}
	e.mon = NewMonitor(cfg, lg, io)
	e.an = NewAnalyze(cfg, lg)
	e.pln = NewPlan(cfg, lg)
	e.exe = NewExecute(lg, io)
	e.stats.ZoneEnergy = map[string]float64{}
	e.stats.EnergyField = map[string]string{}
	engineRef = e
	return e
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
			read, ok, err := e.mon.Latest(ctx, zone)
			if err != nil {
				e.lg.Error("monitor error", "zone", zone, "error", err)
				continue
			}
			if !ok {
				continue
			}
			e.stats.MessagesIn++
			if e.stats.ZoneEnergy == nil {
				e.stats.ZoneEnergy = map[string]float64{}
			}
			if e.stats.EnergyField == nil {
				e.stats.EnergyField = map[string]string{}
			}
			e.stats.ZoneEnergy[zone] = read.ZoneEnergyKWhEpoch
			e.stats.EnergyField[zone] = read.ZoneEnergySource
			res := e.an.Run(zone, read)
			cmds, led := e.pln.Build(zone, read.EpochIndex, read.EpochStart, read.EpochEnd, res)
			if err := e.exe.Do(ctx, zone, cmds, led); err != nil {
				e.lg.Error("execute error", "zone", zone, "error", err)
				continue
			}
			e.stats.CommandsOut += int64(len(cmds))
			e.stats.LedgerWrites++
		}
		e.stats.Loops++
		time.Sleep(interval)
	}
}

func globalStats() Stats {
	if engineRef == nil {
		return Stats{}
	}
	return engineRef.stats
}
