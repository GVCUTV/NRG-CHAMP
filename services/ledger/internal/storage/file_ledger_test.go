// v1
// internal/storage/file_ledger_test.go
package storage

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"nrgchamp/ledger/internal/models"
)

func TestAppendAndVerify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	st, err := NewFileLedger(path, log)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}
	payload := json.RawMessage([]byte(`{"a":1}`))
	ev := &models.Event{Type: "test", ZoneID: "Z1", Source: "unit-test", Payload: payload}
	if _, err := st.Append(ev); err != nil {
		t.Fatalf("append1: %v", err)
	}
	ev2 := &models.Event{Type: "test", ZoneID: "Z2", Source: "unit-test", Payload: payload}
	if _, err := st.Append(ev2); err != nil {
		t.Fatalf("append2: %v", err)
	}
	if err := st.Verify(); err != nil {
		t.Fatalf("verify: %v", err)
	}
}
