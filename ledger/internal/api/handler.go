package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

type Transaction struct {
	TxID      string      `json:"txId"`
	ZoneID    string      `json:"zoneId"`
	Timestamp string      `json:"timestamp"`
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Status    string      `json:"status"`
}

type PaginatedTransactions struct {
	Total        int           `json:"total"`
	Page         int           `json:"page"`
	PageSize     int           `json:"pageSize"`
	Transactions []Transaction `json:"transactions"`
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func listTransactions(w http.ResponseWriter, r *http.Request) {
	// TODO: implement filtering & pagination
	resp := PaginatedTransactions{
		Total:        0,
		Page:         1,
		PageSize:     20,
		Transactions: []Transaction{},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func getTransaction(w http.ResponseWriter, r *http.Request) {
	txId := mux.Vars(r)["txId"]
	// TODO: fetch real transaction
	resp := Transaction{TxID: txId}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
