package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dabump/discogsy/internal/collection"
	"github.com/dabump/discogsy/internal/discogs"
	"github.com/dabump/discogsy/internal/web"
)

const (
	collectionPath = "discogs_collection.json"
	posterDir      = "posters"
)

func main() {
	config, err := discogs.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Discogs sync configured for username %q", config.Username)

	records, err := collection.Load(collectionPath)
	if err != nil {
		log.Fatalf("load collection: %v", err)
	}
	store := web.NewRecordStore(records)
	client := discogs.NewClient(config)
	go discogs.RunEvery(5*time.Minute, client, collectionPath, posterDir, store)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	log.Printf("Discogsy listening on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, web.NewHandler(store, []string{posterDir}, "internal/web/index.html")); err != nil {
		log.Fatal(err)
	}
}
