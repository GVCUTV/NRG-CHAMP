package api

import (
	_ "net/http"

	"github.com/gorilla/mux"
)

func NewRouter() *mux.Router {
	r := mux.NewRouter()

	r.HandleFunc("/health", healthHandler).Methods("GET")
	r.HandleFunc("/batches/latest", getLatestBatch).Methods("GET")

	return r
}
