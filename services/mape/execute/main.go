package main

import (
	"it.uniroma2.dicii/nrg-champ/mape/execute/internal/api"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	router := api.NewRouter()

	logged := handlers.LoggingHandler(os.Stdout, router)
	log.Printf("MAPE-Execute API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, logged))
}
