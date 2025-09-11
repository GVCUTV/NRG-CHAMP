// v1
// internal/plan/plan.go
package plan

import (
	"log/slog"
	"nrg-champ/mape/full/internal/analyze"
	"nrg-champ/mape/full/internal/config"
	"nrg-champ/mape/full/internal/monitor"
)

type Command struct {
	HeaterOn bool   `json:"heaterOn"`
	CoolerOn bool   `json:"coolerOn"`
	FanPct   int    `json:"fanPct"`
	Reason   string `json:"reason"`
}

type Planner struct {
	cfg config.Config
	log *slog.Logger
}

func New(cfg config.Config, log *slog.Logger) *Planner {
	return &Planner{cfg: cfg, log: log.With(slog.String("component", "planner"))}
}

func (p *Planner) Decide(room string, target float64, st monitor.RoomState) *Command {
	ass := analyze.Evaluate(room, st.TempC, target, p.cfg.DeadbandC)
	cmd := &Command{FanPct: 0, Reason: ""}

	if ass.Flags.InBand {
		cmd.HeaterOn, cmd.CoolerOn = false, false
		cmd.FanPct = 0
		cmd.Reason = "in deadband"
		return cmd
	}

	if ass.Flags.BelowTarget {
		cmd.HeaterOn, cmd.CoolerOn = true, false
	} else if ass.Flags.AboveTarget {
		cmd.HeaterOn, cmd.CoolerOn = false, true
	}

	// Simple proportional fan control by magnitude of error
	errAbs := ass.ErrorC
	if errAbs < 0 {
		errAbs = -errAbs
	}
	pct := int(errAbs * p.cfg.Kp)
	if pct > p.cfg.MaxFanPct {
		pct = p.cfg.MaxFanPct
	}
	if pct < 25 {
		pct = 25
	} // minimal circulation when acting
	cmd.FanPct = pct
	cmd.Reason = "P-control toward target"
	return cmd
}
