package main

import (
	"log"
	"net/http"
	"os"

	"github.com/farhan/atlasstore/internal/config"
	"github.com/farhan/atlasstore/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	// Each node gets its own data directory.
	// STORAGE_DATA_DIR lets you run multiple nodes locally by passing different dirs.
	// e.g: STORAGE_DATA_DIR=./data/node1 go run ./cmd/storagenode/
	dataDir := os.Getenv("STORAGE_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data/chunks"
	}
	handler := &storage.NodeHandler{DataDir: dataDir}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chunk", handler.SaveChunk)
	mux.HandleFunc("GET /chunk/{hash}", handler.GetChunk)
	mux.HandleFunc("DELETE /chunk/{hash}", handler.DeleteChunk)
	mux.HandleFunc("GET /health", handler.Health)
	addr := ":" + cfg.StorageNodePort
	log.Printf("Storage node listening on %s | data dir: %s", addr, dataDir)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("storage node crashed: %v", err)
	}
}
