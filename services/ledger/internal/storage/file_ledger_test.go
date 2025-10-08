// v2
// internal/storage/file_ledger_test.go
package storage

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nrgchamp/ledger/internal/models"
)

func TestAppendCreatesGenesisBlockV2(t *testing.T) {
	st, path := newTestLedger(t)
	payload := json.RawMessage([]byte(`{"a":1}`))
	if _, err := st.Append(&models.Event{Type: "test", ZoneID: "Z1", Source: "unit-test", Payload: payload}); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 block, got %d", len(lines))
	}
	var blk models.BlockV2
	if err := json.Unmarshal(lines[0], &blk); err != nil {
		t.Fatalf("unmarshal block: %v", err)
	}
	if blk.Header.Version != models.BlockVersionV2 {
		t.Fatalf("unexpected version: %s", blk.Header.Version)
	}
	if blk.Header.Height != 0 {
		t.Fatalf("unexpected height: %d", blk.Header.Height)
	}
	if blk.Header.PrevHeaderHash != "" {
		t.Fatalf("expected empty prev header hash, got %s", blk.Header.PrevHeaderHash)
	}
	if blk.Header.BlockSize != int64(len(lines[0])) {
		t.Fatalf("unexpected block size: %d", blk.Header.BlockSize)
	}
	if len(blk.Header.Nonce) < 32 {
		t.Fatalf("nonce too short: %d", len(blk.Header.Nonce))
	}
	if len(blk.Data.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(blk.Data.Transactions))
	}
	tx := blk.Data.Transactions[0]
	if tx.PrevHash != "" {
		t.Fatalf("expected empty prev hash for genesis tx")
	}
	hash, err := tx.ComputeHash()
	if err != nil {
		t.Fatalf("compute hash: %v", err)
	}
	if hash != tx.Hash {
		t.Fatalf("hash mismatch")
	}
	dataHash, err := models.ComputeDataHashV2(blk.Data.Transactions)
	if err != nil {
		t.Fatalf("data hash: %v", err)
	}
	if dataHash != blk.Header.DataHash {
		t.Fatalf("data hash mismatch")
	}
	headerHash, err := models.ComputeHeaderHashV2(&blk.Header)
	if err != nil {
		t.Fatalf("header hash: %v", err)
	}
	if headerHash != blk.Header.HeaderHash {
		t.Fatalf("header hash mismatch")
	}
}

func TestVerifyThreeBlockChain(t *testing.T) {
	st, _ := newTestLedger(t)
	payload := json.RawMessage([]byte(`{"a":2}`))
	for i := 0; i < 3; i++ {
		if _, err := st.Append(&models.Event{Type: "test", ZoneID: "Z1", Source: "unit-test", Payload: payload}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	report, err := st.Verify()
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if report.V2Blocks != 3 {
		t.Fatalf("expected 3 v2 blocks, got %d", report.V2Blocks)
	}
	if report.LastHeight != 2 {
		t.Fatalf("expected last height 2, got %d", report.LastHeight)
	}
	if report.V1Events != 0 {
		t.Fatalf("expected 0 v1 events, got %d", report.V1Events)
	}
}

func TestVerifyDetectsDataTamper(t *testing.T) {
	st, path := newTestLedger(t)
	payload := json.RawMessage([]byte(`{"a":3}`))
	for i := 0; i < 2; i++ {
		if _, err := st.Append(&models.Event{Type: "test", ZoneID: "Z1", Source: "unit-test", Payload: payload}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if _, err := st.Verify(); err != nil {
		t.Fatalf("baseline verify: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(content), []byte("\n"))
	if len(lines) < 1 {
		t.Fatalf("expected blocks on disk")
	}
	var blk models.BlockV2
	if err := json.Unmarshal(lines[0], &blk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	blk.Data.Transactions[0].Payload = json.RawMessage([]byte(`{"a":999}`))
	tampered, err := json.Marshal(&blk)
	if err != nil {
		t.Fatalf("marshal tampered: %v", err)
	}
	lines[0] = tampered
	output := bytes.Join(lines, []byte("\n"))
	output = append(output, '\n')
	if err := os.WriteFile(path, output, 0o644); err != nil {
		t.Fatalf("write tampered: %v", err)
	}
	if _, err := st.Verify(); err == nil || !strings.Contains(err.Error(), "dataHash mismatch") {
		t.Fatalf("expected dataHash mismatch, got %v", err)
	}
}

func TestVerifyDetectsHeaderTamper(t *testing.T) {
	st, path := newTestLedger(t)
	payload := json.RawMessage([]byte(`{"a":4}`))
	for i := 0; i < 2; i++ {
		if _, err := st.Append(&models.Event{Type: "test", ZoneID: "Z1", Source: "unit-test", Payload: payload}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(content), []byte("\n"))
	if len(lines) < 2 {
		t.Fatalf("expected at least two blocks")
	}
	var blk models.BlockV2
	if err := json.Unmarshal(lines[1], &blk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	blk.Header.PrevHeaderHash = "badprevhash"
	tampered, err := json.Marshal(&blk)
	if err != nil {
		t.Fatalf("marshal tampered: %v", err)
	}
	lines[1] = tampered
	output := bytes.Join(lines, []byte("\n"))
	output = append(output, '\n')
	if err := os.WriteFile(path, output, 0o644); err != nil {
		t.Fatalf("write tampered: %v", err)
	}
	if _, err := st.Verify(); err == nil || !strings.Contains(err.Error(), "prevHeaderHash mismatch") {
		t.Fatalf("expected prevHeaderHash mismatch, got %v", err)
	}
}

func TestVerifyMixedV1V2File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	legacy := &models.Event{ID: 1, Type: "legacy", ZoneID: "Z0", Source: "test", Timestamp: time.Now().UTC()}
	hash, err := legacy.ComputeHash()
	if err != nil {
		t.Fatalf("compute legacy hash: %v", err)
	}
	legacy.Hash = hash
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	st, err := NewFileLedger(path, log)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}
	payload := json.RawMessage([]byte(`{"a":5}`))
	if _, err := st.Append(&models.Event{Type: "modern", ZoneID: "Z9", Source: "unit-test", Payload: payload}); err != nil {
		t.Fatalf("append modern: %v", err)
	}
	report, err := st.Verify()
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if report.V1Events != 1 {
		t.Fatalf("expected 1 v1 event, got %d", report.V1Events)
	}
	if report.V2Blocks != 1 {
		t.Fatalf("expected 1 v2 block, got %d", report.V2Blocks)
	}
	if report.LastHeight != 0 {
		t.Fatalf("expected last height 0, got %d", report.LastHeight)
	}
}

func newTestLedger(t *testing.T) (*FileLedger, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	st, err := NewFileLedger(path, log)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}
	return st, path
}
