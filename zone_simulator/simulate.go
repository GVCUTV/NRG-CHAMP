// v3
// simulate.go
package main

import (
	"context"
	"github.com/segmentio/kafka-go"
	"strconv"
	"time"
)

// startPhysicsLoop integrates thermal dynamics using consistent physical units.
// It uses a capacity-conductance model, producing realistic heating/cooling effects.
func (s *Simulator) startPhysicsLoop(ctx context.Context) {
	// physical constants (can be tuned or made configurable)
	const (
		Cth       = 5e4  // thermal capacity [J/°C]
		Genv      = 30.0 // base envelope conductance [W/°C]
		GventBase = 20.0 // ventilation conductance base [W/°C]
		EtaHeat   = 1.0  // efficiency for heating
		EtaCool   = 1.0  // efficiency for cooling
	)

	ticker := time.NewTicker(s.cfg.Step)
	s.log.Info("physics loop started", "step", s.cfg.Step.String())
	go func() {
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				s.mu.Lock()
				dt := now.Sub(s.lastE).Seconds()
				if dt <= 0 {
					dt = s.cfg.Step.Seconds()
				}
				// Passive exchange
				qEnv := Genv * (s.tOut - s.tIn)
				// Ventilation proportional to fan level
				ventFactor := float64(s.vent) / 100.0
				qVent := GventBase * ventFactor * (s.tOut - s.tIn)
				// Active heat and cooling
				var qHeat, qCool float64
				if s.heat == ModeOn {
					qHeat = EtaHeat * s.cfg.HeatPowerW
				}
				if s.cool == ModeOn {
					qCool = -EtaCool * s.cfg.CoolPowerW
				}
				// total heat flow (Watts = J/s)
				qTotal := qEnv + qVent + qHeat + qCool
				// temperature change
				s.tIn += (qTotal * dt) / Cth

				s.lastE = now
				s.mu.Unlock()
			case <-ctx.Done():
				s.log.Info("physics loop stopped")
				return
			}
		}
	}()
}

// startPublisher keeps publishing device readings at per-type rates.
func (s *Simulator) startPublisher(ctx context.Context, w *kafka.Writer, deviceID string, devType DeviceType) {
	rate := s.cfg.RateForType(devType)
	if rate <= 0 {
		rate = time.Second
	}
	ticker := time.NewTicker(rate)
	s.log.Info("publisher started", "deviceId", deviceID, "type", devType, "rate", rate.String())
	go func() {
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				tIn, _, heat, cool, vent, hKW, cKW, fKW := s.snapshot()
				switch devType {
				case DeviceTempSensor:
					_ = publish(ctx, s.log, w, Reading{
						DeviceID:   deviceID,
						DeviceType: DeviceTempSensor,
						ZoneID:     s.cfg.ZoneID,
						Timestamp:  now,
						Reading:    TempReading{TempC: tIn},
					})
				case DeviceHeating:
					ar := ActuatorReading{State: string(heat), PowerKW: hKW}
					_ = publish(ctx, s.log, w, Reading{
						DeviceID:   deviceID,
						DeviceType: DeviceHeating,
						ZoneID:     s.cfg.ZoneID,
						Timestamp:  now,
						Reading:    ar,
					})
				case DeviceCooling:
					ar := ActuatorReading{State: string(cool), PowerKW: cKW}
					_ = publish(ctx, s.log, w, Reading{
						DeviceID:   deviceID,
						DeviceType: DeviceCooling,
						ZoneID:     s.cfg.ZoneID,
						Timestamp:  now,
						Reading:    ar,
					})
				case DeviceVentilation:
					ar := ActuatorReading{State: strconv.Itoa(vent), PowerKW: fKW}
					_ = publish(ctx, s.log, w, Reading{
						DeviceID:   deviceID,
						DeviceType: DeviceVentilation,
						ZoneID:     s.cfg.ZoneID,
						Timestamp:  now,
						Reading:    ar,
					})
				}
			case <-ctx.Done():
				s.log.Info("publisher stopped", "deviceId", deviceID, "type", devType)
				return
			}
		}
	}()
}
