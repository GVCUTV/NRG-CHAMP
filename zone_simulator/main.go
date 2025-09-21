// v1
// main.go
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	log "log/slog"
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
		r := make([]byte, 8)
		for i := range r {
			r[i] = byte(now >> (uint(i) * 8))
		}
		copy(b[8:], r)
	}
	// variant bits; see RFC 4122
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b)
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

	// Per-device overrides (key: device UUID -> duration)
	DeviceRates map[string]time.Duration

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
	KafkaBrokers []string
	TopicPrefix  string
}

func (c *SimConfig) DeviceRate(deviceID string, devType DeviceType) time.Duration {
	// per-device override
	if c.DeviceRates != nil {
		if d, ok := c.DeviceRates[deviceID]; ok {
			return d
		}
	}
	// fallback to type-level defaults
	switch devType {
	case DeviceTempSensor:
		return c.SensorRate
	case DeviceHeating:
		return c.HeatRate
	case DeviceCooling:
		return c.CoolRate
	case DeviceVentilation:
		return c.FanRate
	default:
		return c.SensorRate
	}
}

// Reading message sent to Kafka
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

type Simulator struct {
	cfg SimConfig

	// thermal
	mu   sync.Mutex
	tIn  float64
	tOut float64

	// actuator state
	heat HVACMode
	cool HVACMode
	vent int // 0,25,50,75,100

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

	// kafka writer for publishing readings
	writer *kafka.Writer

	// context / stop
	cancel context.CancelFunc
	wg     sync.WaitGroup
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
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

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

	// populate per-device rates from properties (keys like device.<deviceId>.rate=500ms)
	devMap := make(map[string]time.Duration)
	for k, v := range props {
		if strings.HasPrefix(k, "device.") && strings.HasSuffix(k, ".rate") {
			id := strings.TrimSuffix(strings.TrimPrefix(k, "device."), ".rate")
			if d, err := time.ParseDuration(v); err == nil {
				devMap[id] = d
			} else {
				log.Warn("invalid device rate for %s: %v", k, err)
			}
		}
	}
	cfg.DeviceRates = devMap

	// read device ids if present
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

func newKafkaWriter(brokers []string, topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.Hash{}, // ensure messages with same key go to same partition
	}
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
		http.Error(w, "invalid state", http.StatusBadRequest)
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
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Simulator) handleCmdVentilation(w http.ResponseWriter, r *http.Request) {
	type body struct {
		Level int `json:"level"`
	}
	var b body
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if b.Level != 0 && b.Level != 25 && b.Level != 50 && b.Level != 75 && b.Level != 100 {
		http.Error(w, "level must be one of 0,25,50,75,100", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.vent = b.Level
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// Start per-partition consumer for a specific device (actuator)
// This function computes the partition for the device key using crc32 and opens a reader on that partition.
// It reads messages and applies the most recent command for that device.
func (s *Simulator) startPartitionConsumerForDevice(ctx context.Context, deviceID string, devType DeviceType) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		topic := s.cfg.TopicPrefix + "." + s.cfg.ZoneID

		// connect to the first broker to discover partitions
		var conn *kafka.Conn
		var err error
		for _, b := range s.cfg.KafkaBrokers {
			conn, err = kafka.Dial("tcp", b)
			if err == nil {
				break
			} else {
				log.Warn("failed to dial broker %s: %v", b, err)
			}
		}
		if conn == nil {
			log.Error("cannot connect to any kafka broker for consumer")
			return
		}
		defer func(conn *kafka.Conn) {
			err := conn.Close()
			if err != nil {
				log.Warn("failed to close kafka connection: %v", err)
			}
		}(conn)

		partitions, err := conn.ReadPartitions(topic)
		if err != nil || len(partitions) == 0 {
			log.Error("failed to read partitions for topic %s: %v", topic, err)
			return
		}
		// deduplicate partition ids list
		partMap := map[int]struct{}{}
		for _, p := range partitions {
			partMap[p.ID] = struct{}{}
		}
		partIDs := make([]int, 0, len(partMap))
		for id := range partMap {
			partIDs = append(partIDs, id)
		}
		num := uint32(len(partIDs))
		if num == 0 {
			log.Error("no partitions found for topic", topic)
			return
		}
		// compute partition index using CRC32 (same as kafka.Hash balancer)
		crc := crc32.ChecksumIEEE([]byte(deviceID))
		idx := int(crc % num)
		partition := partIDs[idx%len(partIDs)]

		log.Info("Device %s assigned to partition %d for topic %s", deviceID, partition, topic)

		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:   s.cfg.KafkaBrokers,
			Topic:     topic,
			Partition: partition,
			MinBytes:  1,
			MaxBytes:  10e6,
		})
		defer func(reader *kafka.Reader) {
			err := reader.Close()
			if err != nil {
				log.Warn("failed to close kafka reader: %v", err)
			}
		}(reader)

		// Read loop
		for {
			m, err := reader.ReadMessage(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Warn("read error for device %s: %v", deviceID, err)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			var rd Reading
			if err := json.Unmarshal(m.Value, &rd); err != nil {
				log.Warn("invalid message for device %s: %v", deviceID, err)
				continue
			}
			// only process messages addressed to this device
			if rd.DeviceID != deviceID {
				continue
			}
			// apply command based on device type
			switch devType {
			case DeviceHeating:
				// expect { "state": "ON"|"OFF" }
				var ar ActuatorReading
				// attempt to unmarshal reading to actuator structure (it may be small)
				b, _ := json.Marshal(rd.Reading)
				_ = json.Unmarshal(b, &ar)
				if strings.ToUpper(ar.State) == "ON" {
					s.mu.Lock()
					s.heat = ModeOn
					s.mu.Unlock()
				} else if strings.ToUpper(ar.State) == "OFF" {
					s.mu.Lock()
					s.heat = ModeOff
					s.mu.Unlock()
				}
			case DeviceCooling:
				var ar ActuatorReading
				b, _ := json.Marshal(rd.Reading)
				_ = json.Unmarshal(b, &ar)
				if strings.ToUpper(ar.State) == "ON" {
					s.mu.Lock()
					s.cool = ModeOn
					s.mu.Unlock()
				} else if strings.ToUpper(ar.State) == "OFF" {
					s.mu.Lock()
					s.cool = ModeOff
					s.mu.Unlock()
				}
			case DeviceVentilation:
				var ar ActuatorReading
				b, _ := json.Marshal(rd.Reading)
				_ = json.Unmarshal(b, &ar)
				// parse numeric levels
				level := 0
				if ar.State != "" {
					if v, err := strconv.Atoi(ar.State); err == nil {
						level = v
					}
				}
				if level == 0 || level == 25 || level == 50 || level == 75 || level == 100 {
					s.mu.Lock()
					s.vent = level
					s.mu.Unlock()
				}
			}
		}
	}()
}

// simulation loop: integrate thermal model and publish readings
func (s *Simulator) startSimulation(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.cfg.Step)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.mu.Lock()
				dt := now.Sub(s.lastE).Hours()
				if s.lastE.IsZero() {
					dt = s.cfg.Step.Hours()
				}
				// thermal passive
				// simple model: tIn moves towards tOut with factor alpha
				s.tIn += s.cfg.Alpha * (s.tOut - s.tIn) * s.cfg.Step.Seconds()
				// heating/cooling effects
				if s.heat == ModeOn {
					// approximate temperature change proportional to power and dt
					s.tIn += s.cfg.HeatPowerW / 1000.0 * dt * 0.5
					s.heatKWh += (s.cfg.HeatPowerW * dt) / 1000.0
				}
				if s.cool == ModeOn {
					s.tIn -= s.cfg.CoolPowerW / 1000.0 * dt * 0.5
					s.coolKWh += (s.cfg.CoolPowerW * dt) / 1000.0
				}
				// ventilation effect proportional to vent level
				ventEffect := float64(s.vent) / 100.0
				s.tIn += (s.tOut - s.tIn) * (ventEffect * s.cfg.Beta * s.cfg.Step.Seconds())
				// fan energy
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

				// publish sensor reading
				tempRd := Reading{
					DeviceID:   s.tempSensorID,
					DeviceType: DeviceTempSensor,
					ZoneID:     s.cfg.ZoneID,
					Timestamp:  now,
					Reading:    TempReading{TempC: s.tIn},
				}
				_ = s.publish(ctx, tempRd)

				// publish actuator readings (state + power + accumulated energy)
				heatAr := ActuatorReading{State: string(s.heat), PowerW: 0, EnergyKWh: s.heatKWh}
				if s.heat == ModeOn {
					heatAr.PowerW = s.cfg.HeatPowerW
				}
				heatMsg := Reading{DeviceID: s.heatID, DeviceType: DeviceHeating, ZoneID: s.cfg.ZoneID, Timestamp: now, Reading: heatAr}
				_ = s.publish(ctx, heatMsg)

				coolAr := ActuatorReading{State: string(s.cool), PowerW: 0, EnergyKWh: s.coolKWh}
				if s.cool == ModeOn {
					coolAr.PowerW = s.cfg.CoolPowerW
				}
				coolMsg := Reading{DeviceID: s.coolID, DeviceType: DeviceCooling, ZoneID: s.cfg.ZoneID, Timestamp: now, Reading: coolAr}
				_ = s.publish(ctx, coolMsg)

				fanAr := ActuatorReading{State: strconv.Itoa(s.vent), PowerW: fanPower, EnergyKWh: s.fanKWh}
				fanMsg := Reading{DeviceID: s.fanID, DeviceType: DeviceVentilation, ZoneID: s.cfg.ZoneID, Timestamp: now, Reading: fanAr}
				_ = s.publish(ctx, fanMsg)

				s.lastE = now
				s.mu.Unlock()
			}
		}
	}()
}

func main() {
	cfg, err := buildConfig()
	if err != nil {
		log.Error("failed to build config: %v", err)
		os.Exit(1)
	}
	// prepare device IDs
	tempID := cfg.TempSensorID
	if tempID == "" {
		tempID = uuidv4()
	}
	heatID := cfg.HeatID
	if heatID == "" {
		heatID = uuidv4()
	}
	coolID := cfg.CoolID
	if coolID == "" {
		coolID = uuidv4()
	}
	fanID := cfg.FanID
	if fanID == "" {
		fanID = uuidv4()
	}

	// create simulator
	ctx, cancel := context.WithCancel(context.Background())
	sim := &Simulator{
		cfg:          cfg,
		tIn:          cfg.InitialTIn,
		tOut:         cfg.InitialTOut,
		heat:         ModeOff,
		cool:         ModeOff,
		vent:         0,
		heatKWh:      0,
		coolKWh:      0,
		fanKWh:       0,
		lastE:        time.Now(),
		tempSensorID: tempID,
		heatID:       heatID,
		coolID:       coolID,
		fanID:        fanID,
		cancel:       cancel,
	}

	topic := cfg.TopicPrefix + "." + cfg.ZoneID
	sim.writer = newKafkaWriter(cfg.KafkaBrokers, topic)

	// start simulation loop
	sim.startSimulation(ctx)

	// start per-partition consumers for each actuator (heating, cooling, ventilation)
	// they will listen only to the partition where messages for that device land
	sim.startPartitionConsumerForDevice(ctx, sim.heatID, DeviceHeating)
	sim.startPartitionConsumerForDevice(ctx, sim.coolID, DeviceCooling)
	sim.startPartitionConsumerForDevice(ctx, sim.fanID, DeviceVentilation)

	// HTTP handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/health", sim.handleHealth)
	mux.HandleFunc("/status", sim.handleStatus)
	mux.HandleFunc("/cmd/heating", sim.handleCmdHeating)
	mux.HandleFunc("/cmd/cooling", sim.handleCmdCooling)
	mux.HandleFunc("/cmd/ventilation", sim.handleCmdVentilation)

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux}

	// signal handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("starting http server on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server error: %v", err)
			cancel()
		}
	}()

	<-stop
	log.Info("shutdown signal received, stopping...")
	// shutdown http server
	_ = srv.Shutdown(context.Background())
	// cancel simulation & consumers
	cancel()
	// wait goroutines
	sim.wg.Wait()
	log.Info("shutdown complete")
}
