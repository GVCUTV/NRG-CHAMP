// Package internal v9
// file: internal/props.go
package internal

import (
	"bufio"
	log "log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Brokers         []string
	Topics          []string
	Epoch           time.Duration
	MaxPerPartition int
	OffsetsPath     string
	MAPETopic       string
	LedgerTopicTmpl string
	LedgerPartAgg   int
	LedgerPartMAPE  int
	OutlierZ        float64
	LogPath         string
}

func DefaultConfig() Config {
	return Config{Brokers: []string{"kafka:9092"}, Epoch: 500 * time.Millisecond, MaxPerPartition: 1000, OffsetsPath: filepath.Join("data", "offsets.json"), MAPETopic: "agg-to-mape", LedgerTopicTmpl: "zone.ledger.{zone}", LedgerPartAgg: 0, LedgerPartMAPE: 1, OutlierZ: 4.0, LogPath: filepath.Join("data", "aggregator.log")}
}

func LoadProps(path string) Config {
	cfg := DefaultConfig()
	f, err := os.Open(path)
	if err != nil {
		return cfg
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Error("close_failed", "path", path, "err", err)
		}
	}(f)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
		switch k {
		case "brokers":
			cfg.Brokers = splitCSV(v)
		case "topics":
			cfg.Topics = splitCSV(v)
		case "epoch_ms":
			if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
				cfg.Epoch = time.Duration(ms) * time.Millisecond
			}
		case "max_per_partition":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.MaxPerPartition = n
			}
		case "offsets_path":
			cfg.OffsetsPath = v
		case "mape_topic":
			cfg.MAPETopic = v
		case "ledger_topic_template":
			cfg.LedgerTopicTmpl = v
		case "ledger_partition_aggregator":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.LedgerPartAgg = n
			}
		case "ledger_partition_mape":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.LedgerPartMAPE = n
			}
		case "outlier_z":
			if z, err := strconv.ParseFloat(v, 64); err == nil {
				cfg.OutlierZ = z
			}
		case "log_path":
			cfg.LogPath = v
		}
	}
	return cfg
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
