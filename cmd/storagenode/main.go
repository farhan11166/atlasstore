package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"

	"google.golang.org/grpc"

	"github.com/farhan/atlasstore/internal/config"
	"github.com/farhan/atlasstore/internal/storage"
	"github.com/farhan/atlasstore/pkg/pb"
)

func registerWithGateway(gatewayURL, nodeAddress string) {
	reqBody, _ := json.Marshal(map[string]string{
		"address": nodeAddress,
	})

	// We POST to the gateway telling it our address
	resp, err := http.Post(gatewayURL+"/nodes/register", "application/json", bytes.NewBuffer(reqBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Printf("Warning: Failed to register with gateway: %v", err)
	} else {
		log.Println("Successfully registered with Gateway!")
	}
}

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

	addr := ":" + cfg.StorageNodePort

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	handler := &storage.NodeHandler{DataDir: dataDir}
	pb.RegisterStorageNodeServer(grpcServer, handler)

	gatewayURL := "http://localhost:8000"
	// not including http in the addr now
	nodeAddress := "localhost:" + cfg.StorageNodePort
	registerWithGateway(gatewayURL, nodeAddress)

	log.Printf("Storage node (gRPC) listening on %s | data dir: %s", addr, dataDir)

	// 4. Start serving gRPC!
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server crashed: %v", err)
	}

}
