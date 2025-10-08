// v1
// model.go

package main

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

func uuidv4() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		ts := time.Now().UnixNano()
		for i := 0; i < 8; i++ {
			b[i] = byte(ts >> (8 * i))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b)
}

type Simulator struct {
	log *slog.Logger
	cfg SimConfig

	mu   sync.Mutex
	tIn  float64
	tOut float64

	heat HVACMode
	cool HVACMode
	vent int

	heatKWh float64
	coolKWh float64
	fanKWh  float64

	lastE time.Time

	tempSensorID string
	heatID       string
	coolID       string
	fanID        string
}

func (s *Simulator) integrate(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dt := now.Sub(s.lastE).Hours()
	if s.lastE.IsZero() {
		dt = s.cfg.Step.Hours()
	}

	s.tIn += s.cfg.Alpha * (s.tOut - s.tIn) * s.cfg.Step.Seconds()

	if s.heat == ModeOn {
		s.tIn += s.cfg.HeatPowerW / 1000.0 * dt * 0.5
		s.heatKWh += (s.cfg.HeatPowerW * dt) / 1000.0
	}
	if s.cool == ModeOn {
		s.tIn -= s.cfg.CoolPowerW / 1000.0 * dt * 0.5
		s.coolKWh += (s.cfg.CoolPowerW * dt) / 1000.0
	}

	ventEffect := float64(s.vent) / 100.0
	s.tIn += (s.tOut - s.tIn) * (ventEffect * s.cfg.Beta * s.cfg.Step.Seconds())

	var fanPower float64
	switch s.vent {
	case 25:
		fanPower = s.cfg.FanW25
	case 50:
		fanPower = s.cfg.FanW50
	case 75:
		fanPower = s.cfg.FanW75
	case 100:
		fanPower = s.cfg.FanW100
	default:
		fanPower = 0
	}
	s.fanKWh += (fanPower * dt) / 1000.0

	s.lastE = now
}

func (s *Simulator) snapshot() (tIn float64, tOut float64, heat HVACMode, cool HVACMode, vent int, heatKWh, coolKWh, fanKWh float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tIn, s.tOut, s.heat, s.cool, s.vent, s.heatKWh, s.coolKWh, s.fanKWh
}

func (s *Simulator) ventState() string {
	s.mu.Lock()
	v := s.vent
	s.mu.Unlock()
	return strconv.Itoa(v)
}

func (s *Simulator) setHeating(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	up := strings.ToUpper(state)
	if up == "ON" {
		s.heat = ModeOn
	} else if up == "OFF" {
		s.heat = ModeOff
	}
	s.log.Info("heating set", "state", s.heat)
}
func (s *Simulator) setCooling(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	up := strings.ToUpper(state)
	if up == "ON" {
		s.cool = ModeOn
	} else if up == "OFF" {
		s.cool = ModeOff
	}
	s.log.Info("cooling set", "state", s.cool)
}
func (s *Simulator) setVent(level int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if level == 0 || level == 25 || level == 50 || level == 75 || level == 100 {
		s.vent = level
		s.log.Info("vent set", "level", s.vent)
	} else {
		s.log.Warn("invalid vent level", "level", level)
	}
}
