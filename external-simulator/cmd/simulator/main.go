package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type HVACMode string
type FanLevel string

const (
	HVACOff  HVACMode = "OFF"
	HVACHeat HVACMode = "HEAT_ON"
	HVACCool HVACMode = "COOL_ON"

	FanOff  FanLevel = "OFF"
	FanLow  FanLevel = "LOW"
	FanMed  FanLevel = "MEDIUM"
	FanHigh FanLevel = "HIGH"
)

type SimConfig struct {
	ZoneID         string
	Alpha          float64
	Beta           float64
	InitialTIn     float64
	InitialTOut    float64
	Step           time.Duration
	HeatEffect     float64
	CoolEffect     float64
	FanMultipliers map[FanLevel]float64
	ListenAddr     string
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
	return def
}

func getd(m map[string]string, key string, def time.Duration) time.Duration {
	if v, ok := m[key]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

type Simulator struct {
	cfg  SimConfig
	mu   sync.Mutex
	tIn  float64
	tOut float64
	mode HVACMode
	fan  FanLevel
	quit chan struct{}
}

func NewSimulator(cfg SimConfig) *Simulator {
	return &Simulator{
		cfg:  cfg,
		tIn:  cfg.InitialTIn,
		tOut: cfg.InitialTOut,
		mode: HVACOff,
		fan:  FanOff,
		quit: make(chan struct{}),
	}
}

func (s *Simulator) hvacEffect() float64 {
	eff := 0.0
	switch s.mode {
	case HVACHeat:
		eff = s.cfg.HeatEffect
	case HVACCool:
		eff = -s.cfg.CoolEffect
	default:
		eff = 0.0
	}
	mul := s.cfg.FanMultipliers[s.fan]
	return eff * mul
}

func (s *Simulator) step() {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.tIn + s.cfg.Alpha*(s.tOut-s.tIn) + s.cfg.Beta*s.hvacEffect()
	if !math.IsNaN(next) && !math.IsInf(next, 0) {
		s.tIn = next
	}
}

func (s *Simulator) Start() {
	ticker := time.NewTicker(s.cfg.Step)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.step()
			case <-s.quit:
				ticker.Stop()
				return
			}
		}
	}()
}

func (s *Simulator) Stop() {
	close(s.quit)
}

type EnvDTO struct {
	ZoneID    string    `json:"zoneId"`
	TIn       float64   `json:"t_in"`
	TOut      float64   `json:"t_out"`
	HVACState HVACMode  `json:"hvac_state"`
	Fan       FanLevel  `json:"fan"`
	Timestamp time.Time `json:"timestamp"`
}

type ActuatorCmd struct {
	Mode HVACMode `json:"mode"`
	Fan  FanLevel `json:"fan"`
}

func (s *Simulator) handleGetEnv(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resp := EnvDTO{
		ZoneID:    s.cfg.ZoneID,
		TIn:       s.tIn,
		TOut:      s.tOut,
		HVACState: s.mode,
		Fan:       s.fan,
		Timestamp: time.Now().UTC(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Simulator) handlePostActuator(w http.ResponseWriter, r *http.Request) {
	var cmd ActuatorCmd
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch cmd.Mode {
	case HVACOff, HVACHeat, HVACCool:
	default:
		http.Error(w, "invalid mode", http.StatusBadRequest)
		return
	}
	switch cmd.Fan {
	case FanOff, FanLow, FanMed, FanHigh:
	default:
		http.Error(w, "invalid fan", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.mode = cmd.Mode
	s.fan = cmd.Fan
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	propsPath := os.Getenv("SIM_PROPERTIES")
	if propsPath == "" {
		log.Fatal("SIM_PROPERTIES environment variable not set; simulator cannot start.")
	}
	props, err := loadProps(propsPath)
	if err != nil {
		log.Fatalf("cannot load %s: %v", propsPath, err)
	}

	// Ensure required values are available
	if props["zoneId"] == "" || props["listen_addr"] == "" {
		log.Fatal("Both 'zoneId' and 'listenAddr' must be specified in SIM_PROPERTIES")
	}

	cfg := SimConfig{
		ZoneID:      props["zoneId"],
		Alpha:       getf(props, "alpha", 0.02),
		Beta:        getf(props, "beta", 0.5),
		InitialTIn:  getf(props, "initial_t_in", 26.0),
		InitialTOut: getf(props, "initial_t_out", 32.0),
		Step:        getd(props, "step", time.Second),
		HeatEffect:  getf(props, "heat_effect", 0.20),
		CoolEffect:  getf(props, "cool_effect", 0.25),
		FanMultipliers: map[FanLevel]float64{
			FanOff:  0.0,
			FanLow:  getf(props, "fan_multiplier_low", 0.6),
			FanMed:  getf(props, "fan_multiplier_medium", 1.0),
			FanHigh: getf(props, "fan_multiplier_high", 1.4),
		},
		ListenAddr: props["listen_addr"],
	}
	log.Printf("Starting simulator for zone %s on %s", cfg.ZoneID, cfg.ListenAddr)

	sim := NewSimulator(cfg)
	sim.Start()
	defer sim.Stop()

	r := http.NewServeMux()
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("ok"))
		if err != nil {
			log.Printf("Error writing to /health: %v\n", err)
		}
	})
	r.HandleFunc("/env", sim.handleGetEnv)
	r.HandleFunc("/actuator", sim.handlePostActuator)

	httpSrv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("external-simulator listening on %s", cfg.ListenAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
	log.Println("shutting down simulator")
	err = httpSrv.Close()
	if err != nil {
		log.Fatal("error shutting down server: ", err)
	}
	log.Println("simulator stopped")
}
