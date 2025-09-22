// v3
// file: internal/aggregation.go
package internal

import (
	"math"
	"sort"
	"time"
)

// aggregate cleans overhead, removes outliers, and groups by device.
func aggregate(zone string, epoch EpochID, readings []Reading, zThresh float64) AggregatedEpoch {
	// remove obvious outliers via simple z-score on numeric fields (per-field)
	clean := removeOutliers(readings, zThresh)

	byDev := map[string][]Reading{}
	var temps []float64
	var hums []float64

	for _, r := range clean {
		rr := r        // copy
		rr.Extra = nil // drop overhead
		byDev[r.DeviceID] = append(byDev[r.DeviceID], rr)
		if r.Temperature != nil {
			temps = append(temps, *r.Temperature)
		}
		if r.Humidity != nil {
			hums = append(hums, *r.Humidity)
		}
	}

	summary := map[string]float64{}
	if len(temps) > 0 {
		summary["avgTemp"] = mean(temps)
	}
	if len(hums) > 0 {
		summary["avgHumidity"] = mean(hums)
	}

	return AggregatedEpoch{
		ZoneID:     zone,
		Epoch:      epoch,
		ByDevice:   byDev,
		Summary:    summary,
		ProducedAt: time.Now(),
	}
}

func removeOutliers(rs []Reading, z float64) []Reading {
	if z <= 0 {
		return rs
	}
	// compute field-wise mean/std
	var temps, hums []float64
	for _, r := range rs {
		if r.Temperature != nil {
			temps = append(temps, *r.Temperature)
		}
		if r.Humidity != nil {
			hums = append(hums, *r.Humidity)
		}
	}
	mt, st := meanStd(temps)
	mh, sh := meanStd(hums)

	out := make([]Reading, 0, len(rs))
	for _, r := range rs {
		ok := true
		if r.Temperature != nil && st > 0 {
			if math.Abs((*r.Temperature-mt)/st) > z {
				ok = false
			}
		}
		if r.Humidity != nil && sh > 0 {
			if math.Abs((*r.Humidity-mh)/sh) > z {
				ok = false
			}
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

// sortByTime helps ensure deterministic order.
func sortByTime(rs []Reading) {
	sort.Slice(rs, func(i, j int) bool {
		return rs[i].Timestamp.Before(rs[j].Timestamp)
	})
}
