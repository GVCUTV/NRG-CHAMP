// v2
// file: props.go
package internal

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Brokers      []string
	Topics       []string
	PollInterval time.Duration
	BatchSize    int
	OffsetsPath  string
}

func loadConfig() Config {
	path := strings.TrimSpace(os.Getenv("AGG_PROPERTIES"))
	if path == "" {
		path = "/app/aggregator.properties"
	}
	cfg := Config{
		Brokers:      []string{"kafka:9092"},
		Topics:       nil,
		PollInterval: 500 * time.Millisecond,
		BatchSize:    200,
		OffsetsPath:  filepath.Join(dataDir(), "offsets.json"),
	}
	f, err := os.Open(path)
	if err != nil {
		return cfg
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		switch strings.ToLower(k) {
		case "brokers":
			parts := strings.Split(v, ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					out = append(out, p)
				}
			}
			if len(out) > 0 {
				cfg.Brokers = out
			}
		case "topics":
			parts := strings.Split(v, ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					out = append(out, p)
				}
			}
			cfg.Topics = out
		case "poll_interval_ms":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.PollInterval = time.Duration(n) * time.Millisecond
			}
		case "batch_size":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.BatchSize = n
			}
		case "offsets_path":
			if v != "" {
				cfg.OffsetsPath = v
			}
		}
	}
	_ = os.MkdirAll(filepath.Dir(cfg.OffsetsPath), 0o755)
	return cfg
}

func dataDir() string {
	if d := strings.TrimSpace(os.Getenv("AGG_DATA_DIR")); d != "" {
		return d
	}
	return "/app/data"
}
