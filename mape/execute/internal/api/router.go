package api

import (
	"github.com/gorilla/mux"
)

func NewRouter() *mux.Router {
	r := mux.NewRouter()

	r.HandleFunc("/health", healthHandler).Methods("GET")
	r.HandleFunc("/hvac/commands", postHVACCommand).Methods("POST")
	r.HandleFunc("/hvac/status", getHVACStatus).Methods("GET")

	return r
}
