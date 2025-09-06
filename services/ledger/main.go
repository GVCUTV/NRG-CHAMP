package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"it.uniroma2.dicii/nrg-champ/ledger/internal/api"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	router := api.NewRouter()

	logged := handlers.LoggingHandler(os.Stdout, router)
	log.Printf("Blockchain API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, logged))
}
