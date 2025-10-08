// Package internal v7
// plan.go
package internal

import (
	"log/slog"
	"time"
)

// Plan reads per-zone actuator IDs from properties and enforces complementary OFF.
// Additionally, ventilation devices receive VENTILATE with FanPercent when action is HEAT/COOL.
type Plan struct {
	cfg *AppConfig
	lg  *slog.Logger
}

func NewPlan(cfg *AppConfig, lg *slog.Logger) *Plan { return &Plan{cfg: cfg, lg: lg} }

func (p *Plan) Build(zone string, epochIndex int64, epochStart, epochEnd string, res AnalysisResult) ([]PlanCommand, LedgerEvent) {
	acts := p.cfg.Actuators[zone]
	cmds := make([]PlanCommand, 0, len(acts.Heating)+len(acts.Cooling)+len(acts.Ventilation))
	appendCmds := func(ids []string, mode string, fan int, reason string) {
		for _, id := range ids {
			cmds = append(cmds, PlanCommand{
				ZoneID: zone, ActuatorID: id, Mode: mode, FanPercent: fanIf(mode, fan),
				Reason: reason, EpochIndex: epochIndex, IssuedAt: time.Now().UnixMilli(),
			})
		}
	}
	switch res.Action {
	case "HEAT":
		{
			p.lg.Info("plan", "zone", zone, "action", "HEAT", "fan", res.Fan, "heaters", len(acts.Heating), "coolers_off", len(acts.Cooling), "vents", len(acts.Ventilation))
			appendCmds(acts.Heating, "ON", res.Fan, res.Reason)
			appendCmds(acts.Cooling, "OFF", 0, "complementary off (heating active)")
			appendCmds(acts.Ventilation, itoa(res.Fan), res.Fan, res.Reason)
		}
	case "COOL":
		{
			p.lg.Info("plan", "zone", zone, "action", "COOL", "fan", res.Fan, "coolers", len(acts.Cooling), "heaters_off", len(acts.Heating), "vents", len(acts.Ventilation))
			appendCmds(acts.Cooling, "ON", res.Fan, res.Reason)
			appendCmds(acts.Heating, "OFF", 0, "complementary off (cooling active)")
			appendCmds(acts.Ventilation, itoa(res.Fan), res.Fan, res.Reason)
		}
	default:
		{
			p.lg.Info("plan", "zone", zone, "action", "OFF", "all_off", len(acts.Heating)+len(acts.Cooling)+len(acts.Ventilation))
			appendCmds(acts.Heating, "OFF", 0, "within hysteresis")
			appendCmds(acts.Cooling, "OFF", 0, "within hysteresis")
			appendCmds(acts.Ventilation, itoa(0), 0, "within hysteresis")
		}
	}
	p.lg.Info("commands", "list", cmds)
	led := LedgerEvent{
		EpochIndex: epochIndex, ZoneID: zone,
		Planned: "action=" + res.Action + " heaters=" + itoa(len(acts.Heating)) + " coolers=" + itoa(len(acts.Cooling)) + " vents=" + itoa(len(acts.Ventilation)) + " fan=" + itoa(res.Fan),
		TargetC: res.Target, HystC: res.Hyst, DeltaC: res.Delta, Fan: res.Fan, Start: epochStart, End: epochEnd, Timestamp: time.Now().UnixMilli(),
	}
	return cmds, led
}

func fanIf(mode string, fan int) int {
	if mode == "OFF" {
		return 0
	}
	return fan
}
func itoa(i int) string { return fmtInt(i) }
func fmtInt(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	for i > 0 {
		d := i % 10
		s = string(rune('0'+d)) + s
		i /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}
