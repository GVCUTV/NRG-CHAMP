// v0
// services/ledger/internal/public/epoch.go
package public

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// EventTypeEpochPublic identifies the public epoch payload type.
	EventTypeEpochPublic = "epoch.public"
	// SchemaVersionV1 is the initial version for the public epoch schema.
	SchemaVersionV1 = "v1"
)

var (
	hexPattern = regexp.MustCompile(`^[0-9a-f]+$`)
	validPlans = map[string]struct{}{
		"heat": {},
		"cool": {},
		"hold": {},
	}
)

// Epoch represents the minimal, publicly shareable epoch document.
type Epoch struct {
	Type          string             `json:"type"`
	SchemaVersion string             `json:"schemaVersion"`
	ZoneID        string             `json:"zoneId"`
	EpochIndex    int64              `json:"epochIndex"`
	MatchedAt     time.Time          `json:"matchedAt"`
	Block         BlockSummary       `json:"block"`
	Aggregator    AggregatorEnvelope `json:"aggregator"`
	MAPE          MAPESummary        `json:"mape"`
}

// BlockSummary captures the final block that includes the epoch transaction.
type BlockSummary struct {
	Height     int64  `json:"height"`
	HeaderHash string `json:"headerHash"`
	DataHash   string `json:"dataHash"`
}

// AggregatorEnvelope keeps only stable summary metrics for public distribution.
type AggregatorEnvelope struct {
	Summary map[string]float64 `json:"summary"`
}

// MAPESummary exposes the planned HVAC adjustments for the epoch.
type MAPESummary struct {
	Planned string  `json:"planned"`
	TargetC float64 `json:"targetC"`
	DeltaC  float64 `json:"deltaC"`
	Fan     int     `json:"fan"`
}

// Canonical returns a normalized copy with UTC timestamps and lowercase hashes.
func (e Epoch) Canonical() Epoch {
	out := e
	out.Type = strings.TrimSpace(out.Type)
	out.SchemaVersion = strings.TrimSpace(out.SchemaVersion)
	out.ZoneID = strings.TrimSpace(out.ZoneID)
	if !out.MatchedAt.IsZero() {
		out.MatchedAt = out.MatchedAt.UTC()
	}
	out.Block = BlockSummary{
		Height:     out.Block.Height,
		HeaderHash: strings.ToLower(strings.TrimSpace(out.Block.HeaderHash)),
		DataHash:   strings.ToLower(strings.TrimSpace(out.Block.DataHash)),
	}
	out.Aggregator = AggregatorEnvelope{Summary: cloneSummary(out.Aggregator.Summary)}
	out.MAPE = MAPESummary{
		Planned: strings.ToLower(strings.TrimSpace(out.MAPE.Planned)),
		TargetC: out.MAPE.TargetC,
		DeltaC:  out.MAPE.DeltaC,
		Fan:     out.MAPE.Fan,
	}
	return out
}

// Validate ensures the epoch payload satisfies the public schema contract.
func (e Epoch) Validate() error {
	canonical := e.Canonical()
	if canonical.Type != EventTypeEpochPublic {
		return fmt.Errorf("invalid type: %s", canonical.Type)
	}
	if canonical.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported schema version: %s", canonical.SchemaVersion)
	}
	if canonical.ZoneID == "" {
		return errors.New("zoneId is required")
	}
	if canonical.EpochIndex < 0 {
		return errors.New("epochIndex must be non-negative")
	}
	if canonical.MatchedAt.IsZero() {
		return errors.New("matchedAt is required")
	}
	if canonical.MatchedAt.Location() != time.UTC {
		return errors.New("matchedAt must be in UTC")
	}
	if err := canonical.Block.validate(); err != nil {
		return err
	}
	if err := canonical.Aggregator.validate(); err != nil {
		return err
	}
	if err := canonical.MAPE.validate(); err != nil {
		return err
	}
	return nil
}

// MarshalJSON enforces canonical form to guarantee deterministic output.
func (e Epoch) MarshalJSON() ([]byte, error) {
	canonical := e.Canonical()
	type epochAlias Epoch
	return json.Marshal(epochAlias(canonical))
}

func (b BlockSummary) validate() error {
	if b.Height < 0 {
		return errors.New("block.height must be non-negative")
	}
	if b.HeaderHash == "" {
		return errors.New("block.headerHash is required")
	}
	if !hexPattern.MatchString(b.HeaderHash) {
		return fmt.Errorf("block.headerHash must be lowercase hex: %s", b.HeaderHash)
	}
	if b.DataHash == "" {
		return errors.New("block.dataHash is required")
	}
	if !hexPattern.MatchString(b.DataHash) {
		return fmt.Errorf("block.dataHash must be lowercase hex: %s", b.DataHash)
	}
	return nil
}

func (a AggregatorEnvelope) validate() error {
	for k, v := range a.Summary {
		if strings.TrimSpace(k) == "" {
			return errors.New("aggregator.summary keys must be non-empty")
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("aggregator.summary contains invalid value for %s", k)
		}
	}
	return nil
}

func (m MAPESummary) validate() error {
	if _, ok := validPlans[m.Planned]; !ok {
		return fmt.Errorf("mape.planned must be heat, cool, or hold: %s", m.Planned)
	}
	if math.IsNaN(m.TargetC) || math.IsInf(m.TargetC, 0) {
		return errors.New("mape.targetC must be finite")
	}
	if math.IsNaN(m.DeltaC) || math.IsInf(m.DeltaC, 0) {
		return errors.New("mape.deltaC must be finite")
	}
	return nil
}

// MarshalJSON outputs the aggregator summary with stable key ordering.
func (a AggregatorEnvelope) MarshalJSON() ([]byte, error) {
	keys := make([]string, 0, len(a.Summary))
	for k := range a.Summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf := bytes.NewBufferString("{\"summary\":{")
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(strconv.Quote(k))
		buf.WriteByte(':')
		buf.WriteString(formatFloat(a.Summary[k]))
	}
	buf.WriteString("}}")
	return buf.Bytes(), nil
}

func cloneSummary(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]float64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// UnmarshalJSON preserves deterministic decoding for AggregatorEnvelope.
func (a *AggregatorEnvelope) UnmarshalJSON(data []byte) error {
	type alias struct {
		Summary map[string]float64 `json:"summary"`
	}
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	a.Summary = cloneSummary(tmp.Summary)
	return nil
}

// UnmarshalJSON for Epoch ensures canonical normalization on decode.
func (e *Epoch) UnmarshalJSON(data []byte) error {
	type alias Epoch
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	canonical := Epoch(tmp).Canonical()
	*e = canonical
	return nil
}
