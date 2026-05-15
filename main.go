package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
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
	collectionName, err := requiredEnv("VINYL_COLLECTION_NAME")
	if err != nil {
		log.Fatal(err)
	}

	records, err := collection.Load(collectionPath)
	if err != nil {
		log.Fatalf("load collection: %v", err)
	}
	store := web.NewRecordStore(records)
	client := discogs.NewClient(config)
	go discogs.RunEvery(12*time.Hour, client, collectionPath, posterDir, store)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	log.Printf("Discogsy listening on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, web.NewHandler(store, []string{posterDir}, "internal/web/index.html", collectionName)); err != nil {
		log.Fatal(err)
	}
}

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("missing required environment variable: %s", name)
	}
	return value, nil
}
