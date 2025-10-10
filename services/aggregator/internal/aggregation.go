// Package internal v8
// file: internal/aggregation.go
package internal

import (
	"math"
	"sort"
	"time"
)

// aggregate cleans overhead, removes outliers, and groups by device.
func aggregate(zone string, epoch EpochID, readings []Reading, zThresh float64) AggregatedEpoch {
	clean := removeOutliers(readings, zThresh)
	byDev := map[string][]Reading{}
	var temps, powers []float64
	for _, r := range clean {
		rr := r
		rr.Extra = nil
		byDev[r.DeviceID] = append(byDev[r.DeviceID], rr)
		if r.Temperature != nil {
			temps = append(temps, *r.Temperature)
		}
		if r.PowerKW != nil {
			powers = append(powers, *r.PowerKW*1000.0)
		}
	}
	summary := map[string]float64{}
	if len(temps) > 0 {
		summary["avgTemp"] = mean(temps)
	}
	if len(powers) > 0 {
		summary["avgPowerW"] = mean(powers)
	}
	energyTotals := summarizeEnergyByDevice(byDev, epoch)
	if energyTotals.Total > 0 {
		summary["epochEnergyKWh"] = energyTotals.Total
	}
	return AggregatedEpoch{ZoneID: zone, Epoch: epoch, ByDevice: byDev, Summary: summary, ProducedAt: time.Now()}
}

func removeOutliers(rs []Reading, z float64) []Reading {
	if z <= 0 {
		return rs
	}
	var temps, powers []float64
	for _, r := range rs {
		if r.Temperature != nil {
			temps = append(temps, *r.Temperature)
		}
		if r.PowerKW != nil {
			powers = append(powers, *r.PowerKW)
		}
	}
	mt, st := meanStd(temps)
	mp, sp := meanStd(powers)
	out := make([]Reading, 0, len(rs))
	for _, r := range rs {
		ok := true
		if r.Temperature != nil && st > 0 && math.Abs((*r.Temperature-mt)/st) > z {
			ok = false
		}
		if r.PowerKW != nil && sp > 0 && math.Abs((*r.PowerKW-mp)/sp) > z {
			ok = false
		}
		if ok {
			out = append(out, r)
		}
	}
	return out
}

type energyBreakdown struct {
	Total  float64
	ByType map[string]float64
}

func summarizeEnergyByDevice(byDev map[string][]Reading, epoch EpochID) energyBreakdown {
	out := energyBreakdown{ByType: map[string]float64{}}
	for _, readings := range byDev {
		energy := epochEnergyForDevice(readings, epoch)
		if energy <= 0 {
			continue
		}
		dtype := ""
		if len(readings) > 0 {
			dtype = readings[0].DeviceType
		}
		key := energySummaryKey(dtype)
		out.ByType[key] += energy
		out.Total += energy
	}
	return out
}

func energySummaryKey(deviceType string) string {
	switch deviceType {
	case "act_heating":
		return "epochEnergyKWh.heating"
	case "act_cooling":
		return "epochEnergyKWh.cooling"
	case "act_ventilation":
		return "epochEnergyKWh.ventilation"
	case "":
		return "epochEnergyKWh.unknown"
	default:
		return "epochEnergyKWh." + deviceType
	}
}

func epochEnergyForDevice(readings []Reading, epoch EpochID) float64 {
	samples := make([]Reading, 0, len(readings))
	for _, r := range readings {
		if r.PowerKW == nil {
			continue
		}
		samples = append(samples, r)
	}
	if len(samples) == 0 {
		return 0
	}
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Timestamp.Before(samples[j].Timestamp)
	})
	clamp := func(ts time.Time) time.Time {
		if ts.Before(epoch.Start) {
			return epoch.Start
		}
		if ts.After(epoch.End) {
			return epoch.End
		}
		return ts
	}
	var energy float64
	first := samples[0]
	firstStart := clamp(first.Timestamp)
	if firstStart.After(epoch.Start) {
		energy += (*first.PowerKW) * firstStart.Sub(epoch.Start).Hours()
	}
	for i := 0; i < len(samples)-1; i++ {
		start := clamp(samples[i].Timestamp)
		end := clamp(samples[i+1].Timestamp)
		if end.Before(start) {
			continue
		}
		energy += (*samples[i].PowerKW) * end.Sub(start).Hours()
	}
	last := samples[len(samples)-1]
	tailStart := clamp(last.Timestamp)
	if tailStart.Before(epoch.End) {
		energy += (*last.PowerKW) * epoch.End.Sub(tailStart).Hours()
	}
	return energy
}

func mean(a []float64) float64 {
	if len(a) == 0 {
		return 0
	}
	var s float64
	for _, v := range a {
		s += v
	}
	return s / float64(len(a))
}
func meanStd(a []float64) (float64, float64) {
	if len(a) == 0 {
		return 0, 0
	}
	m := mean(a)
	var s float64
	for _, v := range a {
		d := v - m
		s += d * d
	}
	return m, math.Sqrt(s / float64(len(a)))
}
