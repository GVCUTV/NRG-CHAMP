// v1
// config.go

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type DeviceType string

const (
	DeviceTempSensor  DeviceType = "temp_sensor"
	DeviceHeating     DeviceType = "act_heating"
	DeviceCooling     DeviceType = "act_cooling"
	DeviceVentilation DeviceType = "act_ventilation"
)

type HVACMode string

const (
	ModeOff HVACMode = "OFF"
	ModeOn  HVACMode = "ON"
)

type SimConfig struct {
	ZoneID     string
	ListenAddr string

	Alpha       float64
	Beta        float64
	InitialTIn  float64
	InitialTOut float64
	Step        time.Duration

	// Per-type sampling rates (used for publishing)
	SensorRate time.Duration
	HeatRate   time.Duration
	CoolRate   time.Duration
	FanRate    time.Duration

	// Device IDs (can be provided in properties)
	TempSensorID string
	HeatID       string
	CoolID       string
	FanID        string

	// Power ratings (Watts)
	HeatPowerW float64
	CoolPowerW float64
	FanW25     float64
	FanW50     float64
	FanW75     float64
	FanW100    float64

	// Kafka
	KafkaBrokers       []string
	TopicReadingPrefix string
	TopicCommandPrefix string
}

func loadProps(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot load properties file: %w", err)
	}
	lines := strings.Split(string(b), "\n")
	m := map[string]string{}
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") || strings.HasPrefix(ln, "//") {
			continue
		}
		kv := strings.SplitN(ln, "=", 2)
		if len(kv) != 2 {
			continue
		}
		m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	return m, nil
}

func getf(m map[string]string, key string, def float64, log *slog.Logger) float64 {
	if v, ok := m[key]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		log.Warn("invalid float in properties, using default", "key", key, "val", v, "default", def)
	}
	return def
}

func getd(m map[string]string, key string, def time.Duration, log *slog.Logger) time.Duration {
	if v, ok := m[key]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Warn("invalid duration in properties, using default", "key", key, "val", v, "default", def)
	}
	return def
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func buildConfig(log *slog.Logger) (SimConfig, error) {
	propsPath := os.Getenv("SIM_PROPERTIES")
	if propsPath == "" {
		return SimConfig{}, errors.New("SIM_PROPERTIES env var not set")
	}
	props, err := loadProps(propsPath)
	if err != nil {
		return SimConfig{}, err
	}

	zone := props["zoneId"]
	addr := props["listen_addr"]
	if zone == "" || addr == "" {
		return SimConfig{}, errors.New("properties must include zoneId and listen_addr")
	}

	alpha := getf(props, "alpha", 0.02, log)
	beta := getf(props, "beta", 0.5, log)
	tin := getf(props, "initial_t_in", 26.0, log)
	tout := getf(props, "initial_t_out", 32.0, log)
	step := getd(props, "step", time.Second, log)

	// Per-type rates
	sensorRate := getd(props, "sensor_rate", 2*time.Second, log)
	heatRate := getd(props, "heat_rate", 5*time.Second, log)
	coolRate := getd(props, "cool_rate", 5*time.Second, log)
	fanRate := getd(props, "fan_rate", 5*time.Second, log)

	heatW := getf(props, "heat_power_w", 1500, log)
	coolW := getf(props, "cool_power_w", 1200, log)
	f25 := getf(props, "fan_power_w_25", 50, log)
	f50 := getf(props, "fan_power_w_50", 100, log)
	f75 := getf(props, "fan_power_w_75", 150, log)
	f100 := getf(props, "fan_power_w_100", 200, log)

	brokersEnv := os.Getenv("KAFKA_BROKERS")
	if brokersEnv == "" {
		brokersEnv = "kafka:9092"
	}
	readingsPrefix := os.Getenv("TOPIC_READINGS_PREFIX")
	if readingsPrefix == "" {
		readingsPrefix = "device.readings"
	}
	commandPrefix := os.Getenv("TOPIC_COMMANDS_PREFIX")
	if commandPrefix == "" {
		commandPrefix = "zone.commands"
	}

	cfg := SimConfig{
		ZoneID: zone, ListenAddr: addr,
		Alpha: alpha, Beta: beta,
		InitialTIn: tin, InitialTOut: tout, Step: step,
		SensorRate: sensorRate, HeatRate: heatRate, CoolRate: coolRate, FanRate: fanRate,
		HeatPowerW: heatW, CoolPowerW: coolW,
		FanW25: f25, FanW50: f50, FanW75: f75, FanW100: f100,
		KafkaBrokers:       splitCSV(brokersEnv),
		TopicReadingPrefix: readingsPrefix,
		TopicCommandPrefix: commandPrefix,
	}

	if v, ok := props["device.tempSensorId"]; ok && v != "" {
		cfg.TempSensorID = v
	}
	if v, ok := props["device.heatId"]; ok && v != "" {
		cfg.HeatID = v
	}
	if v, ok := props["device.coolId"]; ok && v != "" {
		cfg.CoolID = v
	}
	if v, ok := props["device.fanId"]; ok && v != "" {
		cfg.FanID = v
	}

	return cfg, nil
}

// RateForType picks the right per-type sampling rate.
func (c *SimConfig) RateForType(t DeviceType) time.Duration {
	ret := time.Second
	switch t {
	case DeviceTempSensor:
		ret = c.SensorRate
	case DeviceHeating:
		ret = c.HeatRate
	case DeviceCooling:
		ret = c.CoolRate
	case DeviceVentilation:
		ret = c.FanRate
	}
	return ret
}
