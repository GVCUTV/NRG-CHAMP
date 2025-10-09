// v0
// internal/score/manager_test.go
package score

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"nrgchamp/gamification/internal/ingest"
)

func TestRefreshFiltersWindow(t *testing.T) {
	store := ingest.NewZoneStore(10)
	now := time.Date(2024, 7, 10, 12, 0, 0, 0, time.UTC)

	store.Append("zone-a", ingest.EpochEnergy{ZoneID: "zone-a", EventTime: now.Add(-2 * time.Hour), EnergyKWh: 3})
	store.Append("zone-a", ingest.EpochEnergy{ZoneID: "zone-a", EventTime: now.Add(-26 * time.Hour), EnergyKWh: 5})

	mgr, err := NewManager(store, slog.New(slog.NewTextHandler(io.Discard, nil)), []WindowSpec{{Name: Window24hName}})
	if err != nil {
		t.Fatalf("manager init failed: %v", err)
	}

	mgr.Refresh(now)

	board, ok := mgr.Snapshot(Window24hName)
	if !ok {
		t.Fatalf("expected leaderboard for window %s", Window24hName)
	}
	if len(board.Entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(board.Entries))
	}
	if board.Entries[0].EnergyKWh != 3 {
		t.Fatalf("expected 3 kWh, got %.2f", board.Entries[0].EnergyKWh)
	}
}

func TestRefreshSortsAscending(t *testing.T) {
	store := ingest.NewZoneStore(10)
	now := time.Date(2024, 7, 10, 15, 0, 0, 0, time.UTC)

	store.Append("zone-a", ingest.EpochEnergy{ZoneID: "zone-a", EventTime: now.Add(-time.Hour), EnergyKWh: 5})
	store.Append("zone-b", ingest.EpochEnergy{ZoneID: "zone-b", EventTime: now.Add(-30 * time.Minute), EnergyKWh: 2})
	store.Append("zone-c", ingest.EpochEnergy{ZoneID: "zone-c", EventTime: now.Add(-45 * time.Minute), EnergyKWh: 7})

	mgr, err := NewManager(store, slog.New(slog.NewTextHandler(io.Discard, nil)), []WindowSpec{{Name: Window24hName}})
	if err != nil {
		t.Fatalf("manager init failed: %v", err)
	}

	mgr.Refresh(now)

	board, ok := mgr.Snapshot(Window24hName)
	if !ok {
		t.Fatalf("expected leaderboard for window %s", Window24hName)
	}

	got := []string{board.Entries[0].ZoneID, board.Entries[1].ZoneID, board.Entries[2].ZoneID}
	want := []string{"zone-b", "zone-a", "zone-c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected order %v, got %v", want, got)
		}
	}
}

func TestRefreshStableOnEqualTotals(t *testing.T) {
	store := ingest.NewZoneStore(10)
	now := time.Date(2024, 7, 11, 9, 0, 0, 0, time.UTC)

	store.Append("zone-a", ingest.EpochEnergy{ZoneID: "zone-a", EventTime: now.Add(-15 * time.Minute), EnergyKWh: 4})
	store.Append("zone-b", ingest.EpochEnergy{ZoneID: "zone-b", EventTime: now.Add(-10 * time.Minute), EnergyKWh: 4})
	store.Append("zone-c", ingest.EpochEnergy{ZoneID: "zone-c", EventTime: now.Add(-5 * time.Minute), EnergyKWh: 4})

	mgr, err := NewManager(store, slog.New(slog.NewTextHandler(io.Discard, nil)), []WindowSpec{{Name: Window24hName}})
	if err != nil {
		t.Fatalf("manager init failed: %v", err)
	}

	mgr.Refresh(now)

	board, ok := mgr.Snapshot(Window24hName)
	if !ok {
		t.Fatalf("expected leaderboard for window %s", Window24hName)
	}

	got := []string{board.Entries[0].ZoneID, board.Entries[1].ZoneID, board.Entries[2].ZoneID}
	want := []string{"zone-a", "zone-b", "zone-c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected stable order %v, got %v", want, got)
		}
	}
}

func TestRefreshHandlesEmptyWindow(t *testing.T) {
	store := ingest.NewZoneStore(10)
	now := time.Date(2024, 7, 12, 8, 0, 0, 0, time.UTC)

	mgr, err := NewManager(store, slog.New(slog.NewTextHandler(io.Discard, nil)), []WindowSpec{{Name: Window24hName}})
	if err != nil {
		t.Fatalf("manager init failed: %v", err)
	}

	mgr.Refresh(now)

	board, ok := mgr.Snapshot(Window24hName)
	if !ok {
		t.Fatalf("expected leaderboard for window %s", Window24hName)
	}
	if len(board.Entries) != 0 {
		t.Fatalf("expected zero entries, got %d", len(board.Entries))
	}
}
