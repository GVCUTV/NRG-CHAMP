// v0
// services/ledger/internal/config.go
package internal

import (
	"errors"
	"fmt"
	"strings"

	publicschema "nrgchamp/ledger/internal/public"
)

// PublicPartitioner enumerates supported Kafka partition strategies for public epochs.
type PublicPartitioner = publicschema.Partitioner

const (
	// PublicPartitionerHash routes events using the default hash on message key.
	PublicPartitionerHash PublicPartitioner = publicschema.PartitionerHash
	// PublicPartitionerRoundRobin distributes events evenly without a key.
	PublicPartitionerRoundRobin PublicPartitioner = publicschema.PartitionerRoundRobin
)

// PublicKeyMode represents how the Kafka message key is derived when publishing public epochs.
type PublicKeyMode = publicschema.KeyMode

const (
	// PublicKeyModeZone uses the zone identifier as the Kafka message key.
	PublicKeyModeZone PublicKeyMode = publicschema.KeyModeZone
	// PublicKeyModeEpoch uses the epoch index as the Kafka message key.
	PublicKeyModeEpoch PublicKeyMode = publicschema.KeyModeEpoch
	// PublicKeyModeNone disables keyed publishing.
	PublicKeyModeNone PublicKeyMode = publicschema.KeyModeNone
)

// PublicPublisherConfig defines the knobs required to publish public epoch documents.
type PublicPublisherConfig struct {
	Enabled       bool
	Topic         string
	Brokers       []string
	Acks          int
	Partitioner   PublicPartitioner
	KeyMode       PublicKeyMode
	SchemaVersion string
}

// Validate ensures the configuration is internally consistent before use.
func (c PublicPublisherConfig) Validate() error {
	switch c.Partitioner {
	case PublicPartitionerHash, PublicPartitionerRoundRobin:
	default:
		return fmt.Errorf("unsupported public partitioner: %s", c.Partitioner)
	}
	switch c.KeyMode {
	case PublicKeyModeZone, PublicKeyModeEpoch, PublicKeyModeNone:
	default:
		return fmt.Errorf("unsupported public key mode: %s", c.KeyMode)
	}
	if c.Acks != -1 && c.Acks != 0 && c.Acks != 1 {
		return fmt.Errorf("public acks must be -1, 0, or 1: %d", c.Acks)
	}
	if strings.TrimSpace(c.SchemaVersion) == "" {
		return errors.New("public schema version is required")
	}
	if c.Enabled {
		if strings.TrimSpace(c.Topic) == "" {
			return errors.New("public topic is required when enabled")
		}
		if len(c.Brokers) == 0 {
			return errors.New("at least one public broker is required when enabled")
		}
	}
	return nil
}

// Clone returns a shallow copy so callers can safely mutate the configuration.
func (c PublicPublisherConfig) Clone() PublicPublisherConfig {
	cp := c
	if len(c.Brokers) > 0 {
		cp.Brokers = append([]string(nil), c.Brokers...)
	}
	return cp
}
