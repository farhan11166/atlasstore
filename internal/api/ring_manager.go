package api

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/farhan/atlasstore/internal/db"
	"github.com/farhan/atlasstore/pkg/ring"
)

type RingManager struct {
	DB   *sql.DB
	Ring *ring.HashRing
	mu   sync.RWMutex
}

func NewRingManager(database *sql.DB, vNodes int) *RingManager {
	return &RingManager{
		DB:   database,
		Ring: ring.New(vNodes),
	}
}

func (rm *RingManager) SyncLoop() {
	go func() {
		for {
			nodes, err := db.GetAllNodes(rm.DB)
			if err != nil {
				log.Printf("RingManager: failed to fetch nodes: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			// Add active nodes, remove inactive nodes
			rm.mu.Lock()
			for _, n := range nodes {
				if n.IsActive {
					rm.Ring.AddNode(n.Address)
				} else {
					rm.Ring.RemoveNode(n.Address)
				}
			}
			rm.mu.Unlock()
			time.Sleep(10 * time.Second) // Poll every 10 seconds
		}
	}()
}
