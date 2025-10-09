// v5
// internal/storage/file_ledger_test.go
package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	if _, _, err := st.Append(sampleTransaction("Z1", 0)); err != nil {
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
	if tx.SchemaVersion != models.TransactionSchemaVersionV1 {
		t.Fatalf("unexpected transaction schema version: %s", tx.SchemaVersion)
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

func TestAppendReturnsMetadata(t *testing.T) {
	st, _ := newTestLedger(t)
	tx, meta, err := st.Append(sampleTransaction("ZONE-META", 12))
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if tx == nil {
		t.Fatalf("expected stored transaction clone")
	}
	if meta.Height != 0 {
		t.Fatalf("expected genesis height 0 got %d", meta.Height)
	}
	if meta.HeaderHash == "" || meta.DataHash == "" {
		t.Fatalf("expected metadata hashes to be populated: %#v", meta)
	}
}

func TestVerifyThreeBlockChain(t *testing.T) {
	st, _ := newTestLedger(t)
	for i := 0; i < 3; i++ {
		if _, _, err := st.Append(sampleTransaction("Z1", int64(i))); err != nil {
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
	for i := 0; i < 2; i++ {
		if _, _, err := st.Append(sampleTransaction("Z1", int64(i))); err != nil {
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
	blk.Data.Transactions[0].Aggregator.Summary["targetC"] = 999
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
	for i := 0; i < 2; i++ {
		if _, _, err := st.Append(sampleTransaction("Z1", int64(i))); err != nil {
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

func TestVerifyLegacyV1File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy-ledger.jsonl")
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	now := time.Date(2023, time.December, 31, 23, 0, 0, 0, time.UTC)
	prevHash := ""
	var lines [][]byte
	for i := 0; i < 3; i++ {
		ev := &models.Event{
			ID:        int64(i + 1),
			Type:      "legacy.event",
			ZoneID:    "Z-LEG",
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Source:    "test",
			PrevHash:  prevHash,
		}
		hash, err := ev.ComputeHash()
		if err != nil {
			t.Fatalf("compute hash %d: %v", i, err)
		}
		ev.Hash = hash
		prevHash = hash
		payload, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal legacy event %d: %v", i, err)
		}
		lines = append(lines, payload)
	}
	content := bytes.Join(lines, []byte("\n"))
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write legacy ledger: %v", err)
	}
	st, err := NewFileLedger(path, log)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}
	report, err := st.Verify()
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if report.V1Events != 3 {
		t.Fatalf("expected 3 v1 events, got %d", report.V1Events)
	}
	if report.V2Blocks != 0 {
		t.Fatalf("expected 0 v2 blocks, got %d", report.V2Blocks)
	}
	if report.LastHeight != -1 {
		t.Fatalf("expected last height -1, got %d", report.LastHeight)
	}
}

func TestVerifyPureV2File(t *testing.T) {
	st, path := newTestLedger(t)
	for i := 0; i < 2; i++ {
		if _, _, err := st.Append(sampleTransaction("Z2", int64(i))); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if err := st.file.Close(); err != nil {
		t.Fatalf("close original ledger: %v", err)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	reopened, err := NewFileLedger(path, log)
	if err != nil {
		t.Fatalf("reopen ledger: %v", err)
	}
	report, err := reopened.Verify()
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if report.V1Events != 0 {
		t.Fatalf("expected 0 v1 events, got %d", report.V1Events)
	}
	if report.V2Blocks != 2 {
		t.Fatalf("expected 2 v2 blocks, got %d", report.V2Blocks)
	}
	if report.LastHeight != 1 {
		t.Fatalf("expected last height 1, got %d", report.LastHeight)
	}
}

func TestLoadDefaultsEmptySchemaVersion(t *testing.T) {
	st, path := newTestLedger(t)
	if _, _, err := st.Append(sampleTransaction("Z3", 0)); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := st.file.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 block, got %d", len(lines))
	}
	var blk models.BlockV2
	if err := json.Unmarshal(lines[0], &blk); err != nil {
		t.Fatalf("unmarshal block: %v", err)
	}
	if len(blk.Data.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(blk.Data.Transactions))
	}
	tx := blk.Data.Transactions[0]
	tx.SchemaVersion = ""
	hash, err := tx.ComputeHash()
	if err != nil {
		t.Fatalf("compute hash: %v", err)
	}
	tx.Hash = hash
	dataHash, err := models.ComputeDataHashV2(blk.Data.Transactions)
	if err != nil {
		t.Fatalf("compute data hash: %v", err)
	}
	blk.Header.DataHash = dataHash
	payload, err := finalizeBlockWithoutValidation(&blk)
	if err != nil {
		t.Fatalf("finalize block: %v", err)
	}
	lines[0] = payload
	content := bytes.Join(lines, []byte("\n"))
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	reopened, err := NewFileLedger(path, log)
	if err != nil {
		t.Fatalf("reload ledger: %v", err)
	}
	if len(reopened.transactions) != 1 {
		t.Fatalf("expected 1 transaction in memory, got %d", len(reopened.transactions))
	}
	loaded := reopened.transactions[0]
	if loaded.SchemaVersion != models.TransactionSchemaVersionV1 {
		t.Fatalf("expected schema version v1, got %q", loaded.SchemaVersion)
	}
	if loaded.Hash != tx.Hash {
		t.Fatalf("expected hash %s, got %s", tx.Hash, loaded.Hash)
	}
	if report, err := reopened.Verify(); err != nil {
		t.Fatalf("verify: %v", err)
	} else if report.V2Blocks != 1 {
		t.Fatalf("expected 1 v2 block, got %d", report.V2Blocks)
	}
}

func TestLoadRejectsUnknownTransactionSchema(t *testing.T) {
	st, path := newTestLedger(t)
	if _, _, err := st.Append(sampleTransaction("Z4", 0)); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := st.file.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 block, got %d", len(lines))
	}
	var blk models.BlockV2
	if err := json.Unmarshal(lines[0], &blk); err != nil {
		t.Fatalf("unmarshal block: %v", err)
	}
	if len(blk.Data.Transactions) == 0 {
		t.Fatalf("expected transactions in block")
	}
	blk.Data.Transactions[0].SchemaVersion = "vX"
	payload, err := json.Marshal(&blk)
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}
	lines[0] = payload
	content := bytes.Join(lines, []byte("\n"))
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if _, err := NewFileLedger(path, log); err == nil || !strings.Contains(err.Error(), "unsupported transaction schema version") {
		t.Fatalf("expected schema version error, got %v", err)
	}
}

func TestVerifyLegacyTamperDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy-ledger.jsonl")
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	legacy := []*models.Event{
		{ID: 1, Type: "legacy", ZoneID: "Z-LEG", Timestamp: time.Now().UTC(), Source: "test"},
		{ID: 2, Type: "legacy", ZoneID: "Z-LEG", Timestamp: time.Now().UTC().Add(time.Minute), Source: "test"},
	}
	for i := range legacy {
		if i == 0 {
			legacy[i].PrevHash = ""
		} else {
			legacy[i].PrevHash = legacy[i-1].Hash
		}
		hash, err := legacy[i].ComputeHash()
		if err != nil {
			t.Fatalf("compute hash %d: %v", i, err)
		}
		legacy[i].Hash = hash
	}
	writeEvents := func(events []*models.Event) {
		lines := make([][]byte, len(events))
		for i, ev := range events {
			payload, err := json.Marshal(ev)
			if err != nil {
				t.Fatalf("marshal legacy %d: %v", i, err)
			}
			lines[i] = payload
		}
		content := bytes.Join(lines, []byte("\n"))
		content = append(content, '\n')
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("write legacy: %v", err)
		}
	}
	writeEvents(legacy)
	st, err := NewFileLedger(path, log)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}
	if err := st.file.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
	// Tamper with the second record by breaking its hash link and recomputing its digest.
	legacy[1].PrevHash = "corrupted"
	tamperedHash, err := legacy[1].ComputeHash()
	if err != nil {
		t.Fatalf("compute tampered hash: %v", err)
	}
	legacy[1].Hash = tamperedHash
	writeEvents(legacy)
	if _, err := st.Verify(); err == nil || !strings.Contains(err.Error(), "prevHash mismatch") {
		t.Fatalf("expected prevHash mismatch, got %v", err)
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
	if _, _, err := st.Append(sampleTransaction("Z9", 0)); err != nil {
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

func sampleTransaction(zone string, epoch int64) *models.Transaction {
	matched := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(epoch) * time.Minute)
	start := matched.Add(-5 * time.Minute)
	aggregator := models.AggregatedEpoch{
		SchemaVersion: "v1",
		ZoneID:        zone,
		Epoch: models.EpochWindow{
			Start: start,
			End:   matched,
			Index: epoch,
			Len:   5 * time.Minute,
		},
		Summary:    map[string]float64{"targetC": 21.5},
		ProducedAt: matched.Add(-time.Second),
	}
	mape := models.MAPELedgerEvent{
		SchemaVersion: "v1",
		EpochIndex:    epoch,
		ZoneID:        zone,
		Planned:       "hold",
		TargetC:       21.5,
		Timestamp:     matched.UnixMilli(),
	}
	return &models.Transaction{
		Type:                 "epoch.match",
		SchemaVersion:        models.TransactionSchemaVersionV1,
		ZoneID:               zone,
		EpochIndex:           epoch,
		Aggregator:           aggregator,
		AggregatorReceivedAt: matched.Add(-500 * time.Millisecond),
		MAPE:                 mape,
		MAPEReceivedAt:       matched.Add(-250 * time.Millisecond),
		MatchedAt:            matched,
	}
}

func finalizeBlockWithoutValidation(block *models.BlockV2) ([]byte, error) {
	if block == nil {
		return nil, fmt.Errorf("nil block")
	}
	block.Header.Timestamp = block.Header.Timestamp.UTC()
	block.Header.BlockSize = 0
	block.Header.HeaderHash = ""
	var payload []byte
	for i := 0; i < 5; i++ {
		headerHash, err := models.ComputeHeaderHashV2(&block.Header)
		if err != nil {
			return nil, err
		}
		block.Header.HeaderHash = headerHash
		payload, err = json.Marshal(block)
		if err != nil {
			return nil, err
		}
		size := int64(len(payload))
		if block.Header.BlockSize == size {
			return payload, nil
		}
		block.Header.BlockSize = size
	}
	return nil, fmt.Errorf("failed to finalize block without validation")
}
