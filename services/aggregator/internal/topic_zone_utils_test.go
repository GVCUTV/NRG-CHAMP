package internal

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestExtractZoneFromTopic(t *testing.T) {
	cases := map[string]string{
		"device.readings.zone-A": "zone-A",
		"zone-A":                 "zone-A",
		"a.b.c":                  "c",
	}
	for input, want := range cases {
		if got := extractZoneFromTopic(input); got != want {
			t.Fatalf("extractZoneFromTopic(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDecodeReadingNewSchemaTopicFallback(t *testing.T) {
	payload := []byte(`{"deviceId":"dev-1","deviceType":"act_heating","timestamp":0,"reading":{"powerW":500}}`)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reading, ok := decodeReadingNewSchema(logger, "device.readings.zone-A", payload, time.Unix(0, 0))
	if !ok {
		t.Fatalf("expected decode to succeed")
	}
	if reading.ZoneID != "zone-A" {
		t.Fatalf("expected zone-A, got %q", reading.ZoneID)
	}
	if reading.PowerW == nil || *reading.PowerW != 500 {
		t.Fatalf("expected powerW 500, got %v", reading.PowerW)
	}
	if reading.PowerKW == nil || *reading.PowerKW != 0.5 {
		t.Fatalf("expected powerKW 0.5, got %v", reading.PowerKW)
	}
}
