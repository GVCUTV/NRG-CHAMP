// v9
// services/mape/internal/analyze.go
package internal

import (
	"fmt"
	"log/slog"
	"math"
)

type Analyze struct {
	cfg *AppConfig
	lg  *slog.Logger
	sp  *ZoneSetpoints
}

type AnalysisResult struct {
	HasTemp            bool
	TempC              float64
	Target             float64
	Hyst               float64
	Delta              float64
	Action             string // HEAT/COOL/OFF
	Fan                int
	Reason             string
	ZoneEnergyKWhEpoch float64
	ZoneEnergySource   string
	ActuatorEnergyKWh  map[string]float64
}

func NewAnalyze(cfg *AppConfig, sp *ZoneSetpoints, lg *slog.Logger) *Analyze {
	return &Analyze{cfg: cfg, lg: lg, sp: sp}
}

// Run decides the action based on the zone's avg temperature from the aggregator summary.
func (a *Analyze) Run(zone string, read Reading) AnalysisResult {
	t, ok := a.sp.Get(zone)
	if !ok {
		t = a.cfg.ZoneTargets[zone]
		a.lg.Warn("setpoint missing in store", "zone", zone, "fallback", t)
	}
	h := a.cfg.ZoneHysteresis[zone]
	res := AnalysisResult{Target: t, Hyst: h, HasTemp: true, TempC: read.AvgTempC, ZoneEnergyKWhEpoch: read.ZoneEnergyKWhEpoch, ZoneEnergySource: read.ZoneEnergySource, ActuatorEnergyKWh: cloneEnergyMap(read.ActuatorEnergyKWh)}
	res.Delta = res.TempC - t
	if res.Delta > h {
		res.Action = "COOL"
		res.Fan = pickFan(math.Abs(res.Delta), a.cfg.FanSteps, a.cfg.FanSpeeds)
		res.Reason = fmt.Sprintf("too hot by %.2fC", res.Delta)
		return res
	}
	if res.Delta < -h {
		res.Action = "HEAT"
		res.Fan = pickFan(math.Abs(res.Delta), a.cfg.FanSteps, a.cfg.FanSpeeds)
		res.Reason = fmt.Sprintf("too cold by %.2fC", -res.Delta)
		return res
	}
	res.Action = "OFF"
	res.Fan = 0
	res.Reason = "within hysteresis"
	return res
}

func pickFan(absDelta float64, steps []float64, speeds []int) int {
	for i, s := range steps {
		if absDelta <= s {
			return speeds[i]
		}
	}
	if n := len(speeds); n > 0 {
		return speeds[n-1]
	}
	return 0
}

func cloneEnergyMap(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]float64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
