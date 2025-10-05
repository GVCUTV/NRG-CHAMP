// v1
// simulate.go

package main

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
)

func (s *Simulator) startPhysicsLoop(ctx context.Context) {
	t := time.NewTicker(s.cfg.Step)
	s.log.Info("physics loop started", "step", s.cfg.Step.String())
	go func() {
		defer t.Stop()
		for {
			select {
			case now := <-t.C:
				s.integrate(now)
			case <-ctx.Done():
				s.log.Info("physics loop stopped")
				return
			}
		}
	}()
}

func (s *Simulator) startPublisher(ctx context.Context, w *kafka.Writer, deviceID string, devType DeviceType) {
	rate := s.cfg.RateForType(devType)
	if rate <= 0 {
		rate = time.Second
	}
	t := time.NewTicker(rate)
	s.log.Info("publisher started", "deviceId", deviceID, "type", devType, "rate", rate.String())
	go func() {
		defer t.Stop()
		for {
			select {
			case now := <-t.C:
				tIn, _, heat, cool, vent, hKWh, cKWh, fKWh := s.snapshot()
				switch devType {
				case DeviceTempSensor:
					_ = publish(ctx, s.log, w, Reading{DeviceID: deviceID, DeviceType: DeviceTempSensor, ZoneID: s.cfg.ZoneID, Timestamp: now, Reading: TempReading{TempC: tIn}})
				case DeviceHeating:
					ar := ActuatorReading{State: string(heat), PowerW: 0, EnergyKWh: hKWh}
					if heat == ModeOn {
						ar.PowerW = s.cfg.HeatPowerW
					}
					_ = publish(ctx, s.log, w, Reading{DeviceID: deviceID, DeviceType: DeviceHeating, ZoneID: s.cfg.ZoneID, Timestamp: now, Reading: ar})
				case DeviceCooling:
					ar := ActuatorReading{State: string(cool), PowerW: 0, EnergyKWh: cKWh}
					if cool == ModeOn {
						ar.PowerW = s.cfg.CoolPowerW
					}
					_ = publish(ctx, s.log, w, Reading{DeviceID: deviceID, DeviceType: DeviceCooling, ZoneID: s.cfg.ZoneID, Timestamp: now, Reading: ar})
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
					_ = publish(ctx, s.log, w, Reading{DeviceID: deviceID, DeviceType: DeviceVentilation, ZoneID: s.cfg.ZoneID, Timestamp: now, Reading: ar})
				}
			case <-ctx.Done():
				s.log.Info("publisher stopped", "deviceId", deviceID, "type", devType)
				return
			}
		}
	}()
}
