// v3
// internal/storage/file_ledger.go
package storage

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nrgchamp/ledger/internal/metrics"
	"nrgchamp/ledger/internal/models"
)

var ErrNotFound = errors.New("not found")

type FileLedger struct {
	mu             sync.RWMutex
	path           string
	log            *slog.Logger
	file           *os.File
	writer         *bufio.Writer
	lastID         int64
	lastHash       string
	lastHeight     int64
	lastHeaderHash string
	events         []*models.Event
	transactions   []*models.Transaction
}

func NewFileLedger(path string, log *slog.Logger) (*FileLedger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	fl := &FileLedger{path: path, log: log, file: f, writer: bufio.NewWriter(f), lastHeight: -1}
	if err := fl.load(); err != nil {
		f.Close()
		return nil, err
	}
	return fl, nil
}

func (fl *FileLedger) load() error {
	fl.log.Info("loading", slog.String("path", fl.path))
	if _, err := fl.file.Seek(0, 0); err != nil {
		return err
	}
	fl.events = nil
	fl.transactions = nil
	fl.lastID = 0
	fl.lastHash = ""
	fl.lastHeaderHash = ""
	fl.lastHeight = -1
	scanner := bufio.NewScanner(fl.file)
	var (
		v1Events int
		v2Blocks int
		line     int
	)
	for scanner.Scan() {
		line++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var blk models.BlockV2
		if err := json.Unmarshal(raw, &blk); err == nil && blk.Header.Version == models.BlockVersionV2 {
			defaulted := normalizeTransactionSchemas(&blk, fl.log, line, true)
			if err := blk.Validate(); err != nil {
				return fmt.Errorf("line %d: %w", line, err)
			}
			restoreTransactionSchemas(&blk, defaulted)
			for idx, tx := range blk.Data.Transactions {
				if tx == nil {
					return fmt.Errorf("line %d: block transaction is nil", line)
				}
				if err := fl.validateTransactionChain(tx); err != nil {
					return fmt.Errorf("line %d: %w", line, err)
				}
				storedTx := tx.Clone()
				if storedTx == nil {
					return fmt.Errorf("line %d: failed to clone transaction", line)
				}
				if idx < len(defaulted) && defaulted[idx] {
					storedTx.SchemaVersion = models.TransactionSchemaVersionV1
				}
				ev, err := transactionToEvent(storedTx)
				if err != nil {
					return fmt.Errorf("line %d: %w", line, err)
				}
				fl.transactions = append(fl.transactions, storedTx)
				fl.events = append(fl.events, cloneEvent(ev))
				if storedTx.ID > fl.lastID {
					fl.lastID = storedTx.ID
				}
				fl.lastHash = storedTx.Hash
			}
			fl.lastHeaderHash = blk.Header.HeaderHash
			fl.lastHeight = blk.Header.Height
			v2Blocks++
			continue
		}
		var ev models.Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if err := fl.validateEventChain(&ev); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		ev.Timestamp = ev.Timestamp.UTC()
		stored := cloneEvent(&ev)
		fl.events = append(fl.events, stored)
		if stored.ID > fl.lastID {
			fl.lastID = stored.ID
		}
		fl.lastHash = stored.Hash
		v1Events++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if _, err := fl.file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	fl.writer = bufio.NewWriter(fl.file)
	fl.log.Info("loaded", slog.Int("records", len(fl.events)), slog.Int("v1Events", v1Events), slog.Int("v2Blocks", v2Blocks), slog.Int64("lastID", fl.lastID), slog.Int64("lastHeight", fl.lastHeight))
	return nil
}

func (fl *FileLedger) Append(tx *models.Transaction) (*models.Transaction, error) {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	if tx == nil {
		return nil, fmt.Errorf("transaction must not be nil")
	}
	if tx.SchemaVersion != models.TransactionSchemaVersionV1 {
		return nil, fmt.Errorf("unsupported transaction schema version %q", tx.SchemaVersion)
	}
	fl.lastID++
	tx.ID = fl.lastID
	if tx.MatchedAt.IsZero() {
		tx.MatchedAt = time.Now().UTC()
	} else {
		tx.MatchedAt = tx.MatchedAt.UTC()
	}
	tx.AggregatorReceivedAt = tx.AggregatorReceivedAt.UTC()
	tx.MAPEReceivedAt = tx.MAPEReceivedAt.UTC()
	tx.PrevHash = fl.lastHash
	hash, err := tx.ComputeHash()
	if err != nil {
		return nil, err
	}
	tx.Hash = hash
	stored := tx.Clone()
	if stored == nil {
		return nil, fmt.Errorf("failed to clone transaction")
	}
	block := models.BlockV2{
		Header: models.BlockHeaderV2{
			Version:        models.BlockVersionV2,
			Height:         fl.lastHeight + 1,
			PrevHeaderHash: fl.lastHeaderHash,
			Timestamp:      time.Now().UTC(),
			Nonce:          "",
		},
		Data: models.BlockDataV2{Transactions: []*models.Transaction{stored}},
	}
	dataHash, err := models.ComputeDataHashV2(block.Data.Transactions)
	if err != nil {
		return nil, err
	}
	block.Header.DataHash = dataHash
	nonce, err := newNonce()
	if err != nil {
		return nil, err
	}
	block.Header.Nonce = nonce
	payload, err := finalizeBlock(&block)
	if err != nil {
		return nil, err
	}
	if _, err := fl.writer.Write(payload); err != nil {
		return nil, err
	}
	if err := fl.writer.WriteByte('\n'); err != nil {
		return nil, err
	}
	if err := fl.writer.Flush(); err != nil {
		return nil, err
	}
	if err := fl.file.Sync(); err != nil {
		return nil, err
	}
	ev, err := transactionToEvent(stored)
	if err != nil {
		return nil, err
	}
	fl.lastHash = stored.Hash
	fl.lastHeaderHash = block.Header.HeaderHash
	fl.lastHeight = block.Header.Height
	fl.transactions = append(fl.transactions, stored)
	fl.events = append(fl.events, cloneEvent(ev))
	fl.log.Info("appended block", slog.Int64("height", block.Header.Height), slog.String("headerHash", block.Header.HeaderHash), slog.Int64("transactionID", stored.ID))
	return stored.Clone(), nil
}

func (fl *FileLedger) GetByID(id int64) (*models.Event, error) {
	fl.mu.RLock()
	defer fl.mu.RUnlock()
	for _, e := range fl.events {
		if e.ID == id {
			c := *e
			if e.Payload != nil {
				c.Payload = append([]byte(nil), e.Payload...)
			}
			return &c, nil
		}
	}
	return nil, ErrNotFound
}

func (fl *FileLedger) Query(typ, zoneID, from, to string, page, size int) ([]*models.Event, int) {
	fl.mu.RLock()
	defer fl.mu.RUnlock()
	var tFrom, tTo *time.Time
	if from != "" {
		if tt, err := parseTime(from); err == nil {
			tFrom = &tt
		}
	}
	if to != "" {
		if tt, err := parseTime(to); err == nil {
			tTo = &tt
		}
	}
	filtered := make([]*models.Event, 0, len(fl.events))
	for _, e := range fl.events {
		if typ != "" && !strings.EqualFold(e.Type, typ) {
			continue
		}
		if zoneID != "" && !strings.EqualFold(e.ZoneID, zoneID) {
			continue
		}
		if tFrom != nil && e.Timestamp.Before(*tFrom) {
			continue
		}
		if tTo != nil && e.Timestamp.After(*tTo) {
			continue
		}
		c := *e
		if e.Payload != nil {
			c.Payload = append([]byte(nil), e.Payload...)
		}
		filtered = append(filtered, &c)
	}
	total := len(filtered)
	if size <= 0 {
		size = 50
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * size
	if start >= total {
		return []*models.Event{}, total
	}
	end := start + size
	if end > total {
		end = total
	}
	return filtered[start:end], total
}

type VerifyReport struct {
	V1Events   int   `json:"v1Events"`
	V2Blocks   int   `json:"v2Blocks"`
	LastHeight int64 `json:"lastHeight"`
}

func (fl *FileLedger) Verify() (*VerifyReport, error) {
	fl.mu.RLock()
	defer fl.mu.RUnlock()
	report := &VerifyReport{LastHeight: -1}
	f, err := os.Open(fl.path)
	if err != nil {
		return report, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var (
		prevEventHash  string
		prevHeaderHash string
		prevHeight     int64 = -1
		line           int
	)
	for scanner.Scan() {
		line++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var blk models.BlockV2
		if err := json.Unmarshal(raw, &blk); err == nil && blk.Header.Version == models.BlockVersionV2 {
			defaulted := normalizeTransactionSchemas(&blk, nil, line, false)
			if err := blk.Validate(); err != nil {
				return report, fmt.Errorf("line %d: %w", line, err)
			}
			restoreTransactionSchemas(&blk, defaulted)
			if prevHeight == -1 {
				if blk.Header.Height != 0 {
					return report, fmt.Errorf("line %d: height mismatch", line)
				}
				if blk.Header.PrevHeaderHash != "" {
					return report, fmt.Errorf("line %d: prevHeaderHash mismatch", line)
				}
			} else if blk.Header.Height != prevHeight+1 {
				return report, fmt.Errorf("line %d: height mismatch", line)
			}
			if prevHeight >= 0 && blk.Header.PrevHeaderHash != prevHeaderHash {
				return report, fmt.Errorf("line %d: prevHeaderHash mismatch", line)
			}
			dataHash, err := models.ComputeDataHashV2(blk.Data.Transactions)
			if err != nil {
				return report, fmt.Errorf("line %d: %w", line, err)
			}
			if dataHash != blk.Header.DataHash {
				return report, fmt.Errorf("line %d: dataHash mismatch", line)
			}
			headerHash, err := models.ComputeHeaderHashV2(&blk.Header)
			if err != nil {
				return report, fmt.Errorf("line %d: %w", line, err)
			}
			if headerHash != blk.Header.HeaderHash {
				return report, fmt.Errorf("line %d: headerHash mismatch", line)
			}
			if int64(len(raw)) != blk.Header.BlockSize {
				return report, fmt.Errorf("line %d: blockSize mismatch", line)
			}
			for _, tx := range blk.Data.Transactions {
				if tx == nil {
					return report, fmt.Errorf("line %d: block transaction is nil", line)
				}
				h, err := tx.ComputeHash()
				if err != nil {
					return report, fmt.Errorf("line %d: %w", line, err)
				}
				if h != tx.Hash {
					return report, fmt.Errorf("line %d: transaction hash mismatch id=%d", line, tx.ID)
				}
				if prevEventHash == "" {
					if tx.PrevHash != "" {
						return report, fmt.Errorf("line %d: prevHash mismatch id=%d", line, tx.ID)
					}
				} else if tx.PrevHash != prevEventHash {
					return report, fmt.Errorf("line %d: prevHash mismatch id=%d", line, tx.ID)
				}
				prevEventHash = tx.Hash
			}
			prevHeaderHash = blk.Header.HeaderHash
			prevHeight = blk.Header.Height
			report.V2Blocks++
			report.LastHeight = blk.Header.Height
			continue
		}
		var ev models.Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			return report, fmt.Errorf("line %d: %w", line, err)
		}
		h, err := ev.ComputeHash()
		if err != nil {
			return report, fmt.Errorf("line %d: %w", line, err)
		}
		if h != ev.Hash {
			return report, fmt.Errorf("line %d: hash mismatch id=%d", line, ev.ID)
		}
		if prevEventHash == "" {
			if ev.PrevHash != "" {
				return report, fmt.Errorf("line %d: prevHash mismatch id=%d", line, ev.ID)
			}
		} else if ev.PrevHash != prevEventHash {
			return report, fmt.Errorf("line %d: prevHash mismatch id=%d", line, ev.ID)
		}
		prevEventHash = ev.Hash
		report.V1Events++
	}
	if err := scanner.Err(); err != nil {
		return report, err
	}
	return report, nil
}

func (fl *FileLedger) validateEventChain(ev *models.Event) error {
	if ev == nil {
		return errors.New("nil event")
	}
	expectedPrev := fl.lastHash
	if len(fl.events) == 0 {
		if ev.PrevHash != "" {
			return fmt.Errorf("prevHash mismatch id=%d", ev.ID)
		}
	} else if ev.PrevHash != expectedPrev {
		return fmt.Errorf("prevHash mismatch id=%d", ev.ID)
	}
	h, err := ev.ComputeHash()
	if err != nil {
		return err
	}
	if h != ev.Hash {
		return fmt.Errorf("hash mismatch id=%d", ev.ID)
	}
	return nil
}

func (fl *FileLedger) validateTransactionChain(tx *models.Transaction) error {
	if tx == nil {
		return errors.New("nil transaction")
	}
	expectedPrev := fl.lastHash
	if len(fl.events) == 0 && len(fl.transactions) == 0 {
		if tx.PrevHash != "" {
			return fmt.Errorf("prevHash mismatch id=%d", tx.ID)
		}
	} else if tx.PrevHash != expectedPrev {
		return fmt.Errorf("prevHash mismatch id=%d", tx.ID)
	}
	h, err := tx.ComputeHash()
	if err != nil {
		return err
	}
	if h != tx.Hash {
		return fmt.Errorf("hash mismatch id=%d", tx.ID)
	}
	return nil
}

// normalizeTransactionSchemas promotes empty transaction schema versions to the canonical v1 identifier so legacy records load
// successfully. It records observability signals when requested and returns a bitmap describing which transactions were
// defaulted, allowing callers to restore the original values before performing hash validation.
func normalizeTransactionSchemas(block *models.BlockV2, logger *slog.Logger, line int, countMetric bool) []bool {
	if block == nil {
		return nil
	}
	defaulted := make([]bool, len(block.Data.Transactions))
	for idx, tx := range block.Data.Transactions {
		if tx == nil {
			continue
		}
		if tx.SchemaVersion != "" {
			continue
		}
		if countMetric {
			metrics.IncLedgerLoadTxSchemaEmpty()
		}
		if logger != nil {
			logger.Warn(
				"ledger_default_transaction_schema_version",
				slog.Int("line", line),
				slog.Int("transactionIndex", idx),
				slog.Int64("transactionID", tx.ID),
				slog.Int64("blockHeight", block.Header.Height),
				slog.String("fallback", models.TransactionSchemaVersionV1),
			)
		}
		tx.SchemaVersion = models.TransactionSchemaVersionV1
		defaulted[idx] = true
	}
	return defaulted
}

// restoreTransactionSchemas reverts schemaVersion fields to empty strings for transactions previously defaulted by
// normalizeTransactionSchemas so that hash verification uses the original payload.
func restoreTransactionSchemas(block *models.BlockV2, defaulted []bool) {
	if block == nil {
		return
	}
	for idx, tx := range block.Data.Transactions {
		if tx == nil {
			continue
		}
		if idx >= len(defaulted) || !defaulted[idx] {
			continue
		}
		tx.SchemaVersion = ""
	}
}

func cloneEvent(ev *models.Event) *models.Event {
	if ev == nil {
		return nil
	}
	cp := *ev
	if ev.Payload != nil {
		cp.Payload = append([]byte(nil), ev.Payload...)
	}
	return &cp
}

func transactionToEvent(tx *models.Transaction) (*models.Event, error) {
	if tx == nil {
		return nil, fmt.Errorf("nil transaction")
	}
	record := tx.MatchRecord()
	record.AggregatorReceived = record.AggregatorReceived.UTC()
	record.MAPEReceived = record.MAPEReceived.UTC()
	record.MatchedAt = record.MatchedAt.UTC()
	payload, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal transaction payload: %w", err)
	}
	ev := &models.Event{
		ID:            tx.ID,
		Type:          tx.Type,
		ZoneID:        tx.ZoneID,
		Timestamp:     record.MatchedAt,
		Source:        "ledger.kafka",
		CorrelationID: fmt.Sprintf("%s-%d", tx.ZoneID, tx.EpochIndex),
		Payload:       payload,
		PrevHash:      tx.PrevHash,
		Hash:          tx.Hash,
	}
	return ev, nil
}

func finalizeBlock(block *models.BlockV2) ([]byte, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}
	if err := block.Validate(); err != nil {
		return nil, err
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
	return nil, errors.New("failed to finalize block")
}

func newNonce() (string, error) {
	buf := make([]byte, models.BlockNonceBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func parseTime(s string) (time.Time, error) {
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts.UTC(), nil
	}
	if sec, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(sec, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid time: %s", s)
}
