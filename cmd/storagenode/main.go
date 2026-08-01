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
func startRaftAPI(raftNode *hraft.Raft, fsm *atlasraft.ClusterFSM, apiPort string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/join", func(w http.ResponseWriter, r *http.Request) {
		if raftNode.State() != hraft.Leader {
			http.Error(w, "Not the leader", http.StatusBadGateway)
			return
		}

		nodeID := r.URL.Query().Get("id")
		nodeAddr := r.URL.Query().Get("addr")
		grpcAddr := r.URL.Query().Get("grpc_addr") // NEW

		if nodeID == "" || nodeAddr == "" || grpcAddr == "" {
			http.Error(w, "Missing id, addr, or grpc_addr", http.StatusBadRequest)
			return
		}

		// 1. Add to Raft consensus group
		future := raftNode.AddVoter(hraft.ServerID(nodeID), hraft.ServerAddress(nodeAddr), 0, 0)
		if err := future.Error(); err != nil {
			http.Error(w, fmt.Sprintf("Failed to add voter: %v", err), http.StatusInternalServerError)
			return
		}

		// 2. NEW: Write to the FSM log (since we are the Leader, this works)
		cmd := atlasraft.Command{Type: atlasraft.CmdNodeJoin, Address: grpcAddr}
		data, _ := cmd.Encode()
		if err := raftNode.Apply(data, 5*time.Second).Error(); err != nil {
			http.Error(w, fmt.Sprintf("Failed to apply join to FSM: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("joined successfully"))
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		leaderAddr := ""
		if leader := raftNode.Leader(); leader != "" {
			leaderAddr = string(leader)
		}
		state := map[string]interface{}{
			"leader": leaderAddr,
			"nodes":  fsm.GetNodes(),
		}
		json.NewEncoder(w).Encode(state)
	})
	log.Printf("Raft HTTP API listening on :%s", apiPort)
	go http.ListenAndServe(":"+apiPort, mux)
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
	raftAPIPort := fmt.Sprintf("2%s", cfg.StorageNodePort)
	startRaftAPI(raftNode, fsm, raftAPIPort)
	if len(peers) > 0 {
		go func() {
			joined := false
			for !joined {
				for _, peerAPIAddr := range peers {
					joinURL := fmt.Sprintf("http://%s/join?id=%s&addr=%s&grpc_addr=%s", peerAPIAddr, raftAddr, raftAddr, "localhost:"+cfg.StorageNodePort)
					resp, err := http.Post(joinURL, "application/json", nil)
					if err == nil && resp.StatusCode == http.StatusOK {
						log.Printf("Successfully joined Raft cluster via %s", peerAPIAddr)
						joined = true
						break
					}
				}
				if !joined {
					log.Println("Could not join via any peer, retrying in 2s...")
					time.Sleep(2 * time.Second)
				}
			}
		}()
	}
	if err != nil {
		log.Fatalf("raft start failed: %v", err)
	}

	// Announce this node to the FSM once we are leader or follower
	go func() {
		// Wait until Raft has a leader
		for raftNode.Leader() == "" {
			time.Sleep(500 * time.Millisecond)
		}
		
		// Only announce ourselves directly if WE are the leader (e.g. Node A on bootstrap)
		// If we are a follower, the Leader already added us when we called /join!
		if raftNode.State() == hraft.Leader {
			cmd := atlasraft.Command{Type: atlasraft.CmdNodeJoin, Address: "localhost:" + cfg.StorageNodePort}
			data, _ := cmd.Encode()
			raftNode.Apply(data, 5*time.Second)
			log.Printf("Raft: Leader announced itself to FSM for localhost:%s", cfg.StorageNodePort)
		}
	}()

	// Start gRPC server (unchanged)
	grpcAddr := ":" + cfg.StorageNodePort
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(10*1024*1024), // 10 MB limit
		grpc.MaxSendMsgSize(10*1024*1024),
	)
	handler := &storage.NodeHandler{DataDir: dataDir}
	pb.RegisterStorageNodeServer(grpcServer, handler)

	gatewayURL := "http://localhost:8000"
	registerWithGateway(gatewayURL, "localhost:"+cfg.StorageNodePort)

	log.Printf("Storage node (gRPC) on %s | Raft on %s | data: %s", grpcAddr, raftAddr, dataDir)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server crashed: %v", err)
	}
}
