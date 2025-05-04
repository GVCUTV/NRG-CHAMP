package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"it.uniroma2.dicii/nrg-champ/ledger/internal/blockchain"
	"it.uniroma2.dicii/nrg-champ/ledger/internal/blockchain/api"
	"log"
	"net/http"
)

func main() {
	// Initialize the ledger simulator
	blockchainSimulator := blockchain.NewBlockchainSimulator()

	// Initialize the ledger handler
	blockchainHandler := api.NewBlockchainHandler(blockchainSimulator)

	// Set up the router
	r := mux.NewRouter()

	// Define the API routes
	r.HandleFunc("/api/v1/ledger/transactions", blockchainHandler.CreateTransaction).Methods("POST")
	r.HandleFunc("/api/v1/ledger/transactions", blockchainHandler.ListTransactions).Methods("GET")
	r.HandleFunc("/api/v1/ledger/transactions/{txId}", blockchainHandler.GetTransaction).Methods("GET")
	r.HandleFunc("/health", blockchainHandler.HealthCheck).Methods("GET")

	// Start the server
	fmt.Println("Starting the server on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", r))
}
