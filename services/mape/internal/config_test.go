// v0
// services/mape/internal/config_test.go
package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPropertiesAppliesZoneOverrides(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "mape.properties")
	body := "zones=zone-A,zone-B\n" +
		"target=22.0\n" +
		"target.zone-B=21.5\n" +
		"hysteresis=0.5\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write properties: %v", err)
	}
	cfg := &AppConfig{}
	if err := cfg.loadProperties(path); err != nil {
		t.Fatalf("loadProperties error: %v", err)
	}
	if got, want := cfg.ZoneTargets["zone-A"], 22.0; got != want {
		t.Fatalf("zone-A target mismatch: got %.1f want %.1f", got, want)
	}
	if got, want := cfg.ZoneTargets["zone-B"], 21.5; got != want {
		t.Fatalf("zone-B target mismatch: got %.1f want %.1f", got, want)
	}
	if len(cfg.ZoneTargets) != 2 {
		t.Fatalf("expected 2 zone targets, got %d", len(cfg.ZoneTargets))
	}
}
