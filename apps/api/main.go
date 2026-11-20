package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

var version = "development"

func main() {
	address := os.Getenv("EMBER_LISTEN_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	log.Printf("Ember API %s listening on %s", version, address)
	log.Fatal(http.ListenAndServe(address, httpHandler()))
}

func httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": version,
		})
	})
	return mux
}
