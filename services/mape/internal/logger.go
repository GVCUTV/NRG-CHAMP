// v2
// logger.go
package internal

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"
)

func Init() (*slog.Logger, *os.File) {
	logDir := os.Getenv("LOG_DIR")
	if logDir == "" {
		logDir = "./logs"
	}
	_ = os.MkdirAll(logDir, 0o755)
	fp := filepath.Join(logDir, "mape.log")
	f, err := os.OpenFile(fp, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		lg := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
		lg.Error("log file open failed, stdout only", "err", err)
		return lg, os.Stdout
	}
	mw := ioMulti(f, os.Stdout)
	lg := slog.New(slog.NewTextHandler(mw, &slog.HandlerOptions{}))
	log.SetOutput(mw)
	return lg, f
}

type multi struct {
	ws []interface{ Write([]byte) (int, error) }
}

func (m multi) Write(b []byte) (int, error) {
	n := 0
	for _, w := range m.ws {
		i, err := w.Write(b)
		n += i
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
func ioMulti(ws ...interface{ Write([]byte) (int, error) }) multi { return multi{ws} }
