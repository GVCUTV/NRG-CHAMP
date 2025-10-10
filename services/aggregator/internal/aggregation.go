// Package internal v9
// file: internal/aggregation.go
package internal

import (
	"math"
	"sort"
	"time"
)

// aggregate cleans overhead, removes outliers, and groups by device.
// The zone parameter must be a pure zone identifier such as "zone-A".
func aggregate(zone string, epoch EpochID, readings []Reading, zThresh float64, energyState *EnergyState) AggregatedEpoch {
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
	deviceEnergies, zoneEnergy := computeActuatorEnergy(zone, epoch, byDev, energyState)
	summary["zoneEnergyKWhEpoch"] = zoneEnergy
	return AggregatedEpoch{
		ZoneID:                 zone,
		Epoch:                  epoch,
		ByDevice:               byDev,
		Summary:                summary,
		ProducedAt:             time.Now(),
		ActuatorEnergyKWhEpoch: deviceEnergies,
		ZoneEnergyKWhEpoch:     zoneEnergy,
	}
}

// EnergyState keeps the last known actuator power value to bridge epoch gaps.
type EnergyState struct {
	last map[string]map[string]float64
}

// NewEnergyState creates an empty accumulator.
func NewEnergyState() *EnergyState {
	return &EnergyState{last: map[string]map[string]float64{}}
}

func (s *EnergyState) remember(zone, device string, kw float64) {
	if s == nil {
		return
	}
	if _, ok := s.last[zone]; !ok {
		s.last[zone] = map[string]float64{}
	}
	s.last[zone][device] = kw
}

func (s *EnergyState) recall(zone, device string) (float64, bool) {
	if s == nil {
		return 0, false
	}
	devs, ok := s.last[zone]
	if !ok {
		return 0, false
	}
	kw, ok := devs[device]
	return kw, ok
}

type powerSample struct {
	ts    time.Time
	kw    float64
	order int
}

func computeActuatorEnergy(zone string, epoch EpochID, byDev map[string][]Reading, state *EnergyState) (map[string]float64, float64) {
	energies := map[string]float64{}
	var zoneTotal float64
	processed := map[string]struct{}{}
	for devID, readings := range byDev {
		samples := make([]powerSample, 0, len(readings))
		for idx, r := range readings {
			if kw, ok := readingKW(r); ok {
				samples = append(samples, powerSample{ts: r.Timestamp, kw: kw, order: idx})
			}
		}
		if len(samples) == 0 {
			if lastKW, ok := state.recall(zone, devID); ok {
				energy := lastKW * epoch.Len.Hours()
				energies[devID] = energy
				zoneTotal += energy
			} else {
				energies[devID] = 0
			}
			processed[devID] = struct{}{}
			continue
		}
		sort.SliceStable(samples, func(i, j int) bool {
			if samples[i].ts.Equal(samples[j].ts) {
				return samples[i].order < samples[j].order
			}
			return samples[i].ts.Before(samples[j].ts)
		})
		dedup := make([]powerSample, 0, len(samples))
		for _, s := range samples {
			n := len(dedup)
			if n > 0 && s.ts.Equal(dedup[n-1].ts) {
				dedup[n-1] = s
			} else {
				dedup = append(dedup, s)
			}
		}
		energy := integrateEnergy(epoch, dedup, zone, devID, state)
		energies[devID] = energy
		zoneTotal += energy
		processed[devID] = struct{}{}
	}
	if state != nil {
		if zoneLast, ok := state.last[zone]; ok {
			for devID, lastKW := range zoneLast {
				if _, seen := processed[devID]; seen {
					continue
				}
				energy := lastKW * epoch.Len.Hours()
				energies[devID] = energy
				zoneTotal += energy
			}
		}
	}
	return energies, zoneTotal
}

func integrateEnergy(epoch EpochID, samples []powerSample, zone, devID string, state *EnergyState) float64 {
	start := epoch.Start
	end := epoch.End
	prevTime := start
	prevKW, hasPrev := state.recall(zone, devID)
	if !hasPrev {
		prevKW = 0
	}
	var total float64
	for _, sample := range samples {
		ts := clampTime(sample.ts, start, end)
		if ts.After(prevTime) {
			total += prevKW * ts.Sub(prevTime).Hours()
		}
		prevKW = sample.kw
		prevTime = ts
		if ts.Equal(end) {
			break
		}
	}
	if prevTime.Before(end) {
		total += prevKW * end.Sub(prevTime).Hours()
	}
	if len(samples) > 0 {
		state.remember(zone, devID, samples[len(samples)-1].kw)
	}
	return total
}

func readingKW(r Reading) (float64, bool) {
	switch {
	case r.PowerKW != nil:
		return *r.PowerKW, true
	case r.PowerW != nil:
		return *r.PowerW / 1000.0, true
	default:
		return 0, false
	}
}

func clampTime(ts, start, end time.Time) time.Time {
	if ts.Before(start) {
		return start
	}
	if ts.After(end) {
		return end
	}
	return ts
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
