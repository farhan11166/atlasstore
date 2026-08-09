package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/farhan/atlasstore/internal/db"
	"github.com/farhan/atlasstore/pkg/ring"
)

type RingManager struct {
	DB            *sql.DB
	Ring          *ring.HashRing
	ClusterSecret string
	mu            sync.RWMutex
}

func NewRingManager(database *sql.DB, vNodes int, clusterSecret string) *RingManager {
	return &RingManager{
		DB:            database,
		Ring:          ring.New(vNodes),
		ClusterSecret: clusterSecret,
	}
}

func (rm *RingManager) SyncLoop() {
	go func() {
		for {
			var activeNodes []string

			raftNodes, err := fetchRaftClusterState("http://localhost:29001", rm.ClusterSecret)
			if err == nil {
				activeNodes = raftNodes
				log.Printf("RingManager: synchronized ring from Raft cluster (found %d nodes)", len(activeNodes))
			} else {
				// 2. Fall back to PostgreSQL if Raft is down or starting up
				log.Printf("RingManager: Raft unreachable (%v), falling back to PostgreSQL", err)
				dbNodes, err := db.GetAllNodes(rm.DB)
				if err != nil {
					log.Printf("RingManager: failed to get nodes from DB: %v", err)
					time.Sleep(10 * time.Second)
					continue
				}
				for _, n := range dbNodes {
					if n.IsActive {
						activeNodes = append(activeNodes, n.Address)
					}
				}
			}
			// 3. Rebuild the Hash Ring completely
			rm.mu.Lock()
			rm.Ring = ring.New(50) // Reset the ring
			for _, addr := range activeNodes {
				rm.Ring.AddNode(addr)
			}
			rm.mu.Unlock()
			time.Sleep(10 * time.Second) // Poll every 10 seconds

		}
	}()
}
func fetchRaftClusterState(raftAPIURL string, clusterSecret string) ([]string, error) {
	req, err := http.NewRequest("GET", raftAPIURL+"/state", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Cluster-Token", clusterSecret)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("raft API returned %d", resp.StatusCode)
	}
	var state struct {
		Nodes []string `json:"nodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, err
	}
	return state.Nodes, nil
}
