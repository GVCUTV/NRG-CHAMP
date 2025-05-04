package blockchain

import (
	"fmt"
	"github.com/google/uuid"
	"it.uniroma2.dicii/nrg-champ/ledger/pkg/models"
	"time"
)

// Simulator simulates a simple ledger storage
type Simulator struct {
	LocalLedger []models.Transaction
}

// NewBlockchainSimulator initializes a new BlockchainSimulator instance
func NewBlockchainSimulator() *Simulator {
	return &Simulator{
		LocalLedger: []models.Transaction{},
	}
}

// BuildTransaction creates a new transaction, assigning a UUID, timestamp, and default values
func (bc *Simulator) BuildTransaction(zoneID string, txType string, payload map[string]interface{}) (*models.Transaction, error) {
	txID := uuid.New().String() // Generate a new unique transaction ID
	timestamp := time.Now()

	transaction := models.Transaction{
		TxID:      txID,
		ZoneID:    zoneID,
		Timestamp: timestamp,
		Type:      txType,
		Payload:   payload,
		Status:    "pending", // Default to pending status
	}

	// Append the transaction to the local ledger (simulating ledger)
	bc.LocalLedger = append(bc.LocalLedger, transaction)

	return &transaction, nil
}

// GetTransaction retrieves a transaction by its TxID from the local ledger
func (bc *Simulator) GetTransaction(txID string) (*models.Transaction, error) {
	for _, tx := range bc.LocalLedger {
		if tx.TxID == txID {
			return &tx, nil
		}
	}
	return nil, fmt.Errorf("transaction not found")
}

// ListTransactions lists all transactions from the local ledger
func (bc *Simulator) ListTransactions() []models.Transaction {
	return bc.LocalLedger
}
