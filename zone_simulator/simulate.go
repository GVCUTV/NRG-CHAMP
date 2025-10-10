// v2
// simulate.go
package main

import (
	"context"
	"github.com/segmentio/kafka-go"
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

				// energy meters (convert W*s to kWh)
				if s.heat == ModeOn {
					s.heatKWh += (s.cfg.HeatPowerW * dt / 3600.0) / 1000.0
				}
				if s.cool == ModeOn {
					s.coolKWh += (s.cfg.CoolPowerW * dt / 3600.0) / 1000.0
				}
				var fanW float64
				switch s.vent {
				case 25:
					fanW = s.cfg.FanW25
				case 50:
					fanW = s.cfg.FanW50
				case 75:
					fanW = s.cfg.FanW75
				case 100:
					fanW = s.cfg.FanW100
				}
				s.fanKWh += (fanW * dt / 3600.0) / 1000.0

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
				tIn, _, heat, cool, vent, hKWh, cKWh, fKWh := s.snapshot()
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
					ar := ActuatorReading{State: string(heat), PowerW: 0, EnergyKWh: hKWh}
					if heat == ModeOn {
						ar.PowerW = s.cfg.HeatPowerW
					}
					_ = publish(ctx, s.log, w, Reading{
						DeviceID:   deviceID,
						DeviceType: DeviceHeating,
						ZoneID:     s.cfg.ZoneID,
						Timestamp:  now,
						Reading:    ar,
					})
				case DeviceCooling:
					ar := ActuatorReading{State: string(cool), PowerW: 0, EnergyKWh: cKWh}
					if cool == ModeOn {
						ar.PowerW = s.cfg.CoolPowerW
					}
					_ = publish(ctx, s.log, w, Reading{
						DeviceID:   deviceID,
						DeviceType: DeviceCooling,
						ZoneID:     s.cfg.ZoneID,
						Timestamp:  now,
						Reading:    ar,
					})
				case DeviceVentilation:
					var power float64
					switch vent {
					case 25:
						power = s.cfg.FanW25
					case 50:
						power = s.cfg.FanW50
					case 75:
						power = s.cfg.FanW75
					case 100:
						power = s.cfg.FanW100
					}
					ar := ActuatorReading{State: s.ventState(), PowerW: power, EnergyKWh: fKWh}
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
