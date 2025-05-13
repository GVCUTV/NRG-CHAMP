package api

import (
	"github.com/gorilla/mux"
)

func NewRouter() *mux.Router {
	r := mux.NewRouter()

	r.HandleFunc("/health", healthHandler).Methods("GET")
	r.HandleFunc("/ledger/transactions", listTransactions).Methods("GET")
	r.HandleFunc("/ledger/transactions/{txId}", getTransaction).Methods("GET")

	return r
}
