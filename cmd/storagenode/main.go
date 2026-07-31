package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	hraft "github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"google.golang.org/grpc"

	"github.com/farhan/atlasstore/internal/config"
	"github.com/farhan/atlasstore/internal/storage"
	"github.com/farhan/atlasstore/pkg/pb"
	atlasraft "github.com/farhan/atlasstore/pkg/raft"
)

func registerWithGateway(gatewayURL, nodeAddress string) {
	reqBody, _ := json.Marshal(map[string]string{"address": nodeAddress})
	resp, err := http.Post(gatewayURL+"/nodes/register", "application/json", bytes.NewBuffer(reqBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Printf("Warning: Failed to register with gateway: %v", err)
	} else {
		log.Println("Successfully registered with Gateway!")
	}
}

func startRaft(raftAddr, raftDataDir, nodeGRPCAddr string, peers []string, fsm *atlasraft.ClusterFSM) (*hraft.Raft, error) {
	if err := os.MkdirAll(raftDataDir, 0755); err != nil {
		return nil, fmt.Errorf("create raft dir: %w", err)
	}

	// BoltDB-backed log and stable store
	boltStore, err := raftboltdb.New(raftboltdb.Options{
		Path: raftDataDir + "/raft.db",
	})
	if err != nil {
		return nil, fmt.Errorf("bolt store: %w", err)
	}

	// Snapshot store — keeps snapshots on disk
	snapshots, err := hraft.NewFileSnapshotStore(raftDataDir, 2, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("snapshot store: %w", err)
	}

	// TCP transport — Raft peers talk to each other on this port
	tcpAddr, err := net.ResolveTCPAddr("tcp", raftAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve raft addr: %w", err)
	}
	transport, err := hraft.NewTCPTransport(raftAddr, tcpAddr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("raft transport: %w", err)
	}

	// Raft config
	cfg := hraft.DefaultConfig()
	cfg.LocalID = hraft.ServerID(raftAddr)

	r, err := hraft.NewRaft(cfg, fsm, boltStore, boltStore, snapshots, transport)
	if err != nil {
		return nil, fmt.Errorf("new raft: %w", err)
	}

	// Bootstrap: if no peers, start as a single-node cluster
	if len(peers) == 0 {
		bootstrapCfg := hraft.Configuration{
			Servers: []hraft.Server{
				{ID: hraft.ServerID(raftAddr), Address: hraft.ServerAddress(raftAddr)},
			},
		}
		r.BootstrapCluster(bootstrapCfg)
		log.Printf("Raft: bootstrapped as single-node cluster at %s", raftAddr)
	} else {
		// Wait for a leader then join the cluster
		log.Printf("Raft: joining cluster via peers: %v", peers)
		// (Peer joining happens after gRPC start, handled separately)
	}

	return r, nil
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	dataDir := os.Getenv("STORAGE_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data/chunks"
	}

	// Raft config from env
	raftPort := os.Getenv("RAFT_PORT")
	if raftPort == "" {
		raftPort = fmt.Sprintf("1%s", cfg.StorageNodePort) // e.g. 9001 → 19001
	}
	raftAddr := "localhost:" + raftPort

	raftDataDir := os.Getenv("RAFT_DATA_DIR")
	if raftDataDir == "" {
		raftDataDir = "./raft/" + raftPort
	}

	var peers []string
	if p := os.Getenv("RAFT_PEERS"); p != "" {
		peers = strings.Split(p, ",")
	}

	// Start the Raft FSM
	fsm := atlasraft.NewClusterFSM()
	raftNode, err := startRaft(raftAddr, raftDataDir, "localhost:"+cfg.StorageNodePort, peers, fsm)
	if err != nil {
		log.Fatalf("raft start failed: %v", err)
	}

	// Announce this node to the FSM once we are leader or follower
	go func() {
		// Wait until Raft has a leader
		for raftNode.Leader() == "" {
			time.Sleep(500 * time.Millisecond)
		}
		cmd := atlasraft.Command{Type: atlasraft.CmdNodeJoin, Address: "localhost:" + cfg.StorageNodePort}
		data, _ := cmd.Encode()
		raftNode.Apply(data, 5*time.Second)
		log.Printf("Raft: announced node join for localhost:%s", cfg.StorageNodePort)
	}()

	// Start gRPC server (unchanged)
	grpcAddr := ":" + cfg.StorageNodePort
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	handler := &storage.NodeHandler{DataDir: dataDir}
	pb.RegisterStorageNodeServer(grpcServer, handler)

	gatewayURL := "http://localhost:8000"
	registerWithGateway(gatewayURL, "localhost:"+cfg.StorageNodePort)

	log.Printf("Storage node (gRPC) on %s | Raft on %s | data: %s", grpcAddr, raftAddr, dataDir)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server crashed: %v", err)
	}
}
