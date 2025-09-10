package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	log "log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"
)

// ---- UUIDv4 (no external deps) ----
func uuidv4() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		now := time.Now().UnixNano()
		return fmt.Sprintf("uuid-%d", now)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	hexs := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexs[0:8], hexs[8:12], hexs[12:16], hexs[16:20], hexs[20:32])
}

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

type VentPhase int

type SimConfig struct {
	ZoneID     string
	ListenAddr string

	// Thermal model
	Alpha       float64
	Beta        float64
	InitialTIn  float64
	InitialTOut float64
	Step        time.Duration

	// Sampling rates (per device)
	SensorRate time.Duration
	HeatRate   time.Duration
	CoolRate   time.Duration
	FanRate    time.Duration

	// Power ratings (Watts)
	HeatPowerW float64
	CoolPowerW float64
	FanW25     float64
	FanW50     float64
	FanW75     float64
	FanW100    float64

	// Kafka
	KafkaBrokers []string
	TopicPrefix  string
}

// ---- Utilities ----
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

func getf(m map[string]string, key string, def float64) float64 {
	if v, ok := m[key]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	log.Info("Using default for %s = %v", key, def)
	return def
}

func getd(m map[string]string, key string, def time.Duration) time.Duration {
	if v, ok := m[key]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	log.Info("Using default for %s = %v", key, def)
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

type Simulator struct {
	cfg SimConfig

	// thermal
	mu   sync.Mutex
	tIn  float64
	tOut float64

	// actuator state
	heat HVACMode
	cool HVACMode
	vent VentPhase

	// energy meters (kWh)
	heatKWh float64
	coolKWh float64
	fanKWh  float64

	// last integration time
	lastE time.Time

	// device IDs
	tempSensorID string
	heatID       string
	coolID       string
	fanID        string

	// kafka
	writer *kafka.Writer

	stopC chan struct{}
}

// hvacEffect returns the effect in "degrees per Step" adjusted by ventilation phase
func (s *Simulator) hvacEffect() float64 {
	eff := 0.0
	if s.heat == ModeOn {
		eff += 1.0
	}
	if s.cool == ModeOn {
		eff -= 1.0
	}
	// ventilation multiplier
	mul := 0.0
	switch s.vent {
	case 0:
		mul = 0.0
	case 25:
		mul = 0.25
	case 50:
		mul = 0.50
	case 75:
		mul = 0.75
	case 100:
		mul = 1.0
	default:
		mul = 0.0
	}
	return eff * (1.0 + mul*0.5) // up to +50% effect with max ventilation
}

func (s *Simulator) stepThermal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.tIn + s.cfg.Alpha*(s.tOut-s.tIn) + s.cfg.Beta*s.hvacEffect()
	if !math.IsNaN(next) && !math.IsInf(next, 0) {
		s.tIn = next
	}
}

func (s *Simulator) integrateEnergy(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastE.IsZero() {
		s.lastE = now
		return
	}
	dt := now.Sub(s.lastE).Hours()
	s.lastE = now
	if dt <= 0 {
		return
	}
	if s.heat == ModeOn {
		s.heatKWh += (s.cfg.HeatPowerW * dt) / 1000.0
	}
	if s.cool == ModeOn {
		s.coolKWh += (s.cfg.CoolPowerW * dt) / 1000.0
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
	default:
		fanW = 0.0
	}
	s.fanKWh += (fanW * dt) / 1000.0
}

type Reading struct {
	DeviceID   string     `json:"deviceId"`
	DeviceType DeviceType `json:"deviceType"`
	ZoneID     string     `json:"zoneId"`
	Timestamp  time.Time  `json:"timestamp"`
	Reading    any        `json:"reading"`
}

type TempReading struct {
	TempC float64 `json:"tempC"`
}

type ActuatorReading struct {
	State     string  `json:"state"`     // "ON"/"OFF" or "0/25/50/75/100"
	PowerW    float64 `json:"powerW"`    // instantaneous
	EnergyKWh float64 `json:"energyKWh"` // cumulative
}

func (s *Simulator) topic() string {
	return fmt.Sprintf("%s.%s", s.cfg.TopicPrefix, s.cfg.ZoneID)
}

func (s *Simulator) publish(ctx context.Context, msg Reading) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return s.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(msg.DeviceID),
		Value: b,
		Time:  msg.Timestamp,
	})
}

// HTTP handlers
func (s *Simulator) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("ok"))
	if err != nil {
		log.Error("failed to write health response: %v", err)
		return
	}
}

func (s *Simulator) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resp := map[string]any{
		"zoneId":      s.cfg.ZoneID,
		"t_in":        s.tIn,
		"t_out":       s.tOut,
		"heating":     s.heat,
		"cooling":     s.cool,
		"ventilation": s.vent,
		"energyKWh": map[string]float64{
			"heating": s.heatKWh,
			"cooling": s.coolKWh,
			"fan":     s.fanKWh,
		},
		"devices": map[string]string{
			"tempSensor":  s.tempSensorID,
			"heater":      s.heatID,
			"cooler":      s.coolID,
			"ventilation": s.fanID,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(resp)
	if err != nil {
		log.Error("failed to encode status response: %v", err)
		return
	}
}

func (s *Simulator) handleCmdHeating(w http.ResponseWriter, r *http.Request) {
	type body struct {
		State string `json:"state"`
	}
	var b body
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch strings.ToUpper(b.State) {
	case "ON":
		s.mu.Lock()
		s.heat = ModeOn
		s.mu.Unlock()
	case "OFF":
		s.mu.Lock()
		s.heat = ModeOff
		s.mu.Unlock()
	default:
		http.Error(w, "state must be ON or OFF", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Simulator) handleCmdCooling(w http.ResponseWriter, r *http.Request) {
	type body struct {
		State string `json:"state"`
	}
	var b body
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch strings.ToUpper(b.State) {
	case "ON":
		s.mu.Lock()
		s.cool = ModeOn
		s.mu.Unlock()
	case "OFF":
		s.mu.Lock()
		s.cool = ModeOff
		s.mu.Unlock()
	default:
		http.Error(w, "state must be ON or OFF", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Simulator) handleCmdVentilation(w http.ResponseWriter, r *http.Request) {
	type body struct {
		Level int `json:"level"`
	} // 0,25,50,75,100
	var b body
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch b.Level {
	case 0, 25, 50, 75, 100:
		s.mu.Lock()
		s.vent = VentPhase(b.Level)
		s.mu.Unlock()
	default:
		http.Error(w, "level must be one of 0,25,50,75,100", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Config builder
func buildConfig() (SimConfig, error) {
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

	alpha := getf(props, "alpha", 0.02)
	beta := getf(props, "beta", 0.5)
	tin := getf(props, "initial_t_in", 26.0)
	tout := getf(props, "initial_t_out", 32.0)
	step := getd(props, "step", time.Second)

	sensorRate := getd(props, "sensor_rate", 2*time.Second)
	heatRate := getd(props, "heat_rate", 5*time.Second)
	coolRate := getd(props, "cool_rate", 5*time.Second)
	fanRate := getd(props, "fan_rate", 5*time.Second)

	heatW := getf(props, "heat_power_w", 1500)
	coolW := getf(props, "cool_power_w", 1200)
	f25 := getf(props, "fan_power_w_25", 50)
	f50 := getf(props, "fan_power_w_50", 100)
	f75 := getf(props, "fan_power_w_75", 150)
	f100 := getf(props, "fan_power_w_100", 200)

	brokersEnv := os.Getenv("KAFKA_BROKERS")
	if brokersEnv == "" {
		brokersEnv = "kafka:9092"
	}
	prefix := os.Getenv("TOPIC_PREFIX")
	if prefix == "" {
		prefix = "device.readings"
	}

	cfg := SimConfig{
		ZoneID:     zone,
		ListenAddr: addr,
		Alpha:      alpha, Beta: beta,
		InitialTIn: tin, InitialTOut: tout, Step: step,
		SensorRate: sensorRate, HeatRate: heatRate, CoolRate: coolRate, FanRate: fanRate,
		HeatPowerW: heatW, CoolPowerW: coolW,
		FanW25: f25, FanW50: f50, FanW75: f75, FanW100: f100,
		KafkaBrokers: splitCSV(brokersEnv),
		TopicPrefix:  prefix,
	}
	return cfg, nil
}

func newKafkaWriter(brokers []string, topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		RequiredAcks:           kafka.RequireAll,
		Balancer:               &kafka.LeastBytes{},
		AllowAutoTopicCreation: true,
		BatchTimeout:           10 * time.Millisecond,
	}
}

func main() {
	cfg, err := buildConfig()
	if err != nil {
		log.Error("configuration error: %v", err)
		os.Exit(1)
	}

	log.Info("Starting room-simulator zone=%s listen=%s brokers=%v topic=%s",
		cfg.ZoneID, cfg.ListenAddr, cfg.KafkaBrokers, cfg.TopicPrefix+"."+cfg.ZoneID)

	sim := &Simulator{
		cfg:          cfg,
		tIn:          cfg.InitialTIn,
		tOut:         cfg.InitialTOut,
		heat:         ModeOff,
		cool:         ModeOff,
		vent:         VentPhase(0),
		tempSensorID: uuidv4(),
		heatID:       uuidv4(),
		coolID:       uuidv4(),
		fanID:        uuidv4(),
		stopC:        make(chan struct{}),
	}

	topic := sim.topic()
	sim.writer = newKafkaWriter(cfg.KafkaBrokers, topic)
	defer func(writer *kafka.Writer) {
		err := writer.Close()
		if err != nil {
			log.Error("failed to close kafka writer: %v", err)
		}
	}(sim.writer)

	// Thermal integration loop
	go func() {
		t := time.NewTicker(cfg.Step)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				sim.stepThermal()
				sim.integrateEnergy(time.Now())
			case <-sim.stopC:
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())

	// Sampling loops per device
	// Temperature sensor
	go func() {
		t := time.NewTicker(cfg.SensorRate)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				sim.mu.Lock()
				temp := sim.tIn
				sim.mu.Unlock()
				msg := Reading{
					DeviceID:   sim.tempSensorID,
					DeviceType: DeviceTempSensor,
					ZoneID:     cfg.ZoneID,
					Timestamp:  time.Now().UTC(),
					Reading:    TempReading{TempC: temp},
				}
				if err := sim.publish(ctx, msg); err != nil {
					log.Warn("publish temp failed: %v", err)
				}
			case <-sim.stopC:
				return
			}
		}
	}()

	// Heating meter
	go func() {
		t := time.NewTicker(cfg.HeatRate)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				sim.mu.Lock()
				on := sim.heat == ModeOn
				p := 0.0
				if on {
					p = cfg.HeatPowerW
				}
				kwh := sim.heatKWh
				sim.mu.Unlock()
				msg := Reading{
					DeviceID:   sim.heatID,
					DeviceType: DeviceHeating,
					ZoneID:     cfg.ZoneID,
					Timestamp:  time.Now().UTC(),
					Reading:    ActuatorReading{State: map[bool]string{true: "ON", false: "OFF"}[on], PowerW: p, EnergyKWh: kwh},
				}
				if err := sim.publish(ctx, msg); err != nil {
					log.Warn("publish heat failed: %v", err)
				}
			case <-sim.stopC:
				return
			}
		}
	}()

	// Cooling meter
	go func() {
		t := time.NewTicker(cfg.CoolRate)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				sim.mu.Lock()
				on := sim.cool == ModeOn
				p := 0.0
				if on {
					p = cfg.CoolPowerW
				}
				kwh := sim.coolKWh
				sim.mu.Unlock()
				msg := Reading{
					DeviceID:   sim.coolID,
					DeviceType: DeviceCooling,
					ZoneID:     cfg.ZoneID,
					Timestamp:  time.Now().UTC(),
					Reading:    ActuatorReading{State: map[bool]string{true: "ON", false: "OFF"}[on], PowerW: p, EnergyKWh: kwh},
				}
				if err := sim.publish(ctx, msg); err != nil {
					log.Warn("publish cool failed: %v", err)
				}
			case <-sim.stopC:
				return
			}
		}
	}()

	// Ventilation meter
	go func() {
		t := time.NewTicker(cfg.FanRate)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				sim.mu.Lock()
				level := int(sim.vent)
				var p float64
				switch level {
				case 25:
					p = cfg.FanW25
				case 50:
					p = cfg.FanW50
				case 75:
					p = cfg.FanW75
				case 100:
					p = cfg.FanW100
				default:
					p = 0.0
				}
				kwh := sim.fanKWh
				sim.mu.Unlock()
				msg := Reading{
					DeviceID:   sim.fanID,
					DeviceType: DeviceVentilation,
					ZoneID:     cfg.ZoneID,
					Timestamp:  time.Now().UTC(),
					Reading:    ActuatorReading{State: fmt.Sprintf("%d", level), PowerW: p, EnergyKWh: kwh},
				}
				if err := sim.publish(ctx, msg); err != nil {
					log.Warn("publish fan failed: %v", err)
				}
			case <-sim.stopC:
				return
			}
		}
	}()

	// HTTP Server for health/status and commands
	mux := http.NewServeMux()
	mux.HandleFunc("/health", sim.handleHealth)
	mux.HandleFunc("/status", sim.handleStatus)
	mux.HandleFunc("/actuators/heating", sim.handleCmdHeating)
	mux.HandleFunc("/actuators/cooling", sim.handleCmdCooling)
	mux.HandleFunc("/actuators/ventilation", sim.handleCmdVentilation)

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("room-simulator listening on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server error: %v", err)
		}
	}()

	// graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
	log.Info("shutting down...")
	close(sim.stopC)
	cancel()
	_ = srv.Close()
	_ = sim.writer.Close()
	log.Info("bye")
}
