// v0
// services/mape/internal/setpoints.go
package internal

import (
	"errors"
	"fmt"
	"sync"
)

// ErrUnknownZone is returned when a setpoint operation references a zone that is not tracked.
var ErrUnknownZone = errors.New("unknown zoneId")

// ErrSetpointRange indicates that the provided setpoint falls outside the permitted range.
var ErrSetpointRange = errors.New("setpoint outside configured range")

// ZoneSetpoints stores per-zone temperature setpoints protected by a RWMutex to permit
// concurrent reads from the Analyze/Plan pipeline while HTTP handlers update values.
// The structure also tracks the allowable range so that HTTP validation can be shared
// with other callers (e.g., property reload logic).
type ZoneSetpoints struct {
	mu     sync.RWMutex
	zones  map[string]struct{}
	values map[string]float64
	min    float64
	max    float64
}

// NewZoneSetpoints builds the runtime setpoint store from the parsed configuration. Each
// zone must have an initial value within the configured range so that the planner never
// operates on undefined data.
func NewZoneSetpoints(zones []string, defaults map[string]float64, min, max float64) (*ZoneSetpoints, error) {
	if len(zones) == 0 {
		return nil, fmt.Errorf("setpoints: no zones configured")
	}
	sp := &ZoneSetpoints{
		zones:  make(map[string]struct{}, len(zones)),
		values: make(map[string]float64, len(zones)),
		min:    min,
		max:    max,
	}
	for _, zone := range zones {
		sp.zones[zone] = struct{}{}
		val, ok := defaults[zone]
		if !ok {
			return nil, fmt.Errorf("setpoints: missing initial value for zone %s", zone)
		}
		if val < min || val > max {
			return nil, fmt.Errorf("setpoints: zone %s initial %.2f outside %.2f..%.2f", zone, val, min, max)
		}
		sp.values[zone] = val
	}
	return sp, nil
}

// Get returns the current setpoint for the requested zone. The boolean indicates whether
// the zone was known to the store.
func (s *ZoneSetpoints) Get(zone string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.values[zone]
	return v, ok
}

// All returns a copy of the current setpoints. Callers receive their own map so they can
// safely marshal results without affecting the underlying store.
func (s *ZoneSetpoints) All() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]float64, len(s.values))
	for z, v := range s.values {
		out[z] = v
	}
	return out
}

// Set updates the setpoint for the provided zone after validating that the value stays in
// range. Errors are wrapped with sentinel values so HTTP handlers can translate them into
// correct status codes.
func (s *ZoneSetpoints) Set(zone string, value float64) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.zones[zone]; !ok {
		return 0, fmt.Errorf("%w: %s", ErrUnknownZone, zone)
	}
	if value < s.min || value > s.max {
		return 0, fmt.Errorf("%w: %.2f", ErrSetpointRange, value)
	}
	s.values[zone] = value
	return value, nil
}

// Reset replaces all zone setpoints with the provided defaults. The helper is used when
// properties are reloaded so that the runtime store mirrors the latest configuration. Any
// validation failure leaves the previous values untouched.
func (s *ZoneSetpoints) Reset(defaults map[string]float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for zone := range s.zones {
		val, ok := defaults[zone]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownZone, zone)
		}
		if val < s.min || val > s.max {
			return fmt.Errorf("%w: %.2f", ErrSetpointRange, val)
		}
	}
	for zone := range s.zones {
		s.values[zone] = defaults[zone]
	}
	return nil
}

// Range exposes the allowable bounds so that HTTP validation can present user-friendly
// error messages without duplicating configuration knowledge.
func (s *ZoneSetpoints) Range() (float64, float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.min, s.max
}
