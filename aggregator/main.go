// v0
// aggregator/main.go

package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"it.uniroma2.dicii/nrg-champ/aggregator/internal/api"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	router := api.NewRouter()

	logged := handlers.LoggingHandler(os.Stdout, router)
	log.Printf("Aggregator API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, logged))
}
