// v3
// file: internal/server.go
package internal

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"
)

// StartCmd parses flags and starts the epoch runner.
func StartCmd() error {
	propsPath := flag.String("props", "./aggregator.properties", "Path to properties file")
	flag.Parse()

	cfg := LoadProps(*propsPath)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return Start(ctx, logger, cfg, IO{})
}
