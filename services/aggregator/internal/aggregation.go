// Package internal v8
// file: internal/aggregation.go
package internal

import (
	"math"
	"time"
)

// aggregate cleans overhead, removes outliers, and groups by device.
func aggregate(zone string, epoch EpochID, readings []Reading, zThresh float64) AggregatedEpoch {
	clean := removeOutliers(readings, zThresh)
	byDev := map[string][]Reading{}
	var temps, powers, energies []float64
	for _, r := range clean {
		rr := r
		rr.Extra = nil
		byDev[r.DeviceID] = append(byDev[r.DeviceID], rr)
		if r.Temperature != nil {
			temps = append(temps, *r.Temperature)
		}
		if r.PowerW != nil {
			powers = append(powers, *r.PowerW)
		}
		if r.EnergyKWh != nil {
			energies = append(energies, *r.EnergyKWh)
		}
	}
	summary := map[string]float64{}
	if len(temps) > 0 {
		summary["avgTemp"] = mean(temps)
	}
	if len(powers) > 0 {
		summary["avgPowerW"] = mean(powers)
	}
	if len(energies) > 0 {
		summary["avgEnergyKWh"] = mean(energies)
	}
	return AggregatedEpoch{ZoneID: zone, Epoch: epoch, ByDevice: byDev, Summary: summary, ProducedAt: time.Now()}
}

func removeOutliers(rs []Reading, z float64) []Reading {
	if z <= 0 {
		return rs
	}
	var temps, powers, energies []float64
	for _, r := range rs {
		if r.Temperature != nil {
			temps = append(temps, *r.Temperature)
		}
		if r.PowerW != nil {
			powers = append(powers, *r.PowerW)
		}
		if r.EnergyKWh != nil {
			energies = append(energies, *r.EnergyKWh)
		}
	}
	mt, st := meanStd(temps)
	mp, sp := meanStd(powers)
	me, se := meanStd(energies)
	out := make([]Reading, 0, len(rs))
	for _, r := range rs {
		ok := true
		if r.Temperature != nil && st > 0 && math.Abs((*r.Temperature-mt)/st) > z {
			ok = false
		}
		if r.PowerW != nil && sp > 0 && math.Abs((*r.PowerW-mp)/sp) > z {
			ok = false
		}
		if r.EnergyKWh != nil && se > 0 && math.Abs((*r.EnergyKWh-me)/se) > z {
			ok = false
		}
		if ok {
			out = append(out, r)
		}
	}
	return out
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
