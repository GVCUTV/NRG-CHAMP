// Package internal v0
// file: internal/aggregation_energy_test.go
package internal

import (
	"math"
	"testing"
	"time"
)

func TestEnergySingleReadingExtendsToEnd(t *testing.T) {
	state := NewEnergyState()
	start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	epoch := EpochID{Start: start, End: start.Add(10 * time.Minute), Len: 10 * time.Minute}
	kw := 2.0
	r := makeActuatorReading("zoneA", "dev1", start.Add(2*time.Minute), kw)
	agg := aggregate("zoneA", epoch, []Reading{r}, 0, state)
	got := agg.ActuatorEnergyKWhEpoch["dev1"]
	want := kw * (8.0 / 60.0)
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("expected %.6f kWh, got %.6f", want, got)
	}
	if math.Abs(agg.ZoneEnergyKWhEpoch-want) > 1e-6 {
		t.Fatalf("expected zone total %.6f kWh, got %.6f", want, agg.ZoneEnergyKWhEpoch)
	}
}

func TestEnergyMultipleReadingsIrregular(t *testing.T) {
	state := NewEnergyState()
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	epoch := EpochID{Start: start, End: start.Add(10 * time.Minute), Len: 10 * time.Minute}
	r1 := makeActuatorReading("zoneA", "dev1", start, 2.0)
	r2 := makeActuatorReading("zoneA", "dev1", start.Add(4*time.Minute), 1.0)
	r3 := makeActuatorReading("zoneA", "dev1", start.Add(7*time.Minute), 3.0)
	agg := aggregate("zoneA", epoch, []Reading{r1, r2, r3}, 0, state)
	want := 2.0*(4.0/60.0) + 1.0*(3.0/60.0) + 3.0*(3.0/60.0)
	got := agg.ActuatorEnergyKWhEpoch["dev1"]
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("expected %.6f kWh, got %.6f", want, got)
	}
	if math.Abs(agg.ZoneEnergyKWhEpoch-want) > 1e-6 {
		t.Fatalf("expected zone total %.6f kWh, got %.6f", want, agg.ZoneEnergyKWhEpoch)
	}
}

func TestEnergyGapUsesCarryover(t *testing.T) {
	state := NewEnergyState()
	base := time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC)
	prev := EpochID{Start: base.Add(-10 * time.Minute), End: base, Len: 10 * time.Minute}
	prevReading := makeActuatorReading("zoneA", "dev1", prev.End.Add(-time.Minute), 1.5)
	_ = aggregate("zoneA", prev, []Reading{prevReading}, 0, state)

	epoch := EpochID{Start: base, End: base.Add(10 * time.Minute), Len: 10 * time.Minute}
	nowReading := makeActuatorReading("zoneA", "dev1", base.Add(3*time.Minute), 2.0)
	agg := aggregate("zoneA", epoch, []Reading{nowReading}, 0, state)
	want := 1.5*(3.0/60.0) + 2.0*(7.0/60.0)
	got := agg.ActuatorEnergyKWhEpoch["dev1"]
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("expected %.6f kWh with carryover, got %.6f", want, got)
	}
}

func TestEnergyNoReadingsUsesCarryoverAcrossEpoch(t *testing.T) {
	state := NewEnergyState()
	base := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	prev := EpochID{Start: base.Add(-10 * time.Minute), End: base, Len: 10 * time.Minute}
	prevReading := makeActuatorReading("zoneA", "dev1", prev.End.Add(-time.Minute), 1.0)
	_ = aggregate("zoneA", prev, []Reading{prevReading}, 0, state)

	epoch := EpochID{Start: base, End: base.Add(10 * time.Minute), Len: 10 * time.Minute}
	agg := aggregate("zoneA", epoch, nil, 0, state)
	want := 1.0 * (10.0 / 60.0)
	got := agg.ActuatorEnergyKWhEpoch["dev1"]
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("expected %.6f kWh with carryover only, got %.6f", want, got)
	}
}

func TestEnergyNoReadingsNoCarryover(t *testing.T) {
	state := NewEnergyState()
	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	epoch := EpochID{Start: start, End: start.Add(10 * time.Minute), Len: 10 * time.Minute}
	agg := aggregate("zoneX", epoch, nil, 0, state)
	if len(agg.ActuatorEnergyKWhEpoch) != 0 {
		t.Fatalf("expected no actuator entries, got %#v", agg.ActuatorEnergyKWhEpoch)
	}
	if agg.ZoneEnergyKWhEpoch != 0 {
		t.Fatalf("expected zero zone energy, got %.6f", agg.ZoneEnergyKWhEpoch)
	}
}

func makeActuatorReading(zone, dev string, ts time.Time, kw float64) Reading {
	w := kw * 1000
	return Reading{
		ZoneID:     zone,
		DeviceID:   dev,
		DeviceType: "act_heating",
		Timestamp:  ts,
		PowerKW:    &kw,
		PowerW:     &w,
	}
}
