package api

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"it.uniroma2.dicii/nrg-champ/ledger/internal/blockchain"
	"net/http"
)

// HealthCheckResponse represents health check endpoint response
type HealthCheckResponse struct {
	Status string `json:"status"`
}

// HealthCheck handles the health check endpoint
func (bh *BlockchainHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := HealthCheckResponse{
		Status: "Service is running correctly",
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// BlockchainHandler contains the ledger simulator instance
type BlockchainHandler struct {
	BlockchainSimulator *blockchain.Simulator
}

// NewBlockchainHandler initializes the handler with a ledger simulator
func NewBlockchainHandler(bc *blockchain.Simulator) *BlockchainHandler {
	return &BlockchainHandler{
		BlockchainSimulator: bc,
	}
}

// CreateTransaction handles creating new transactions
func (bh *BlockchainHandler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	var payload map[string]interface{}
	// Parse the incoming request body into the payload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	zoneID := r.URL.Query().Get("zoneId") // Query param for the zone
	txType := "sensor"                    // Default type: "sensor" or "command" based on your system

	// Create the transaction
	transaction, err := bh.BlockchainSimulator.BuildTransaction(zoneID, txType, payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the newly created transaction as JSON
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(transaction)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// ListTransactions handles retrieving all transactions from the ledger
func (bh *BlockchainHandler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	transactions := bh.BlockchainSimulator.ListTransactions()

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(transactions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// GetTransaction handles retrieving a single transaction by TxID
func (bh *BlockchainHandler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	txID := mux.Vars(r)["txId"]

	transaction, err := bh.BlockchainSimulator.GetTransaction(txID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(transaction)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
