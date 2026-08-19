package api

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/farhan/atlasstore/internal/db"
)

func StartHealthChecker(database *sql.DB, storageClient *StorageClient) {
	go func() {
		for {
			nodes, err := db.GetAllNodes(database)
			if err != nil {
				log.Printf("Health check failed to get nodes: %v", err)
				time.Sleep(10 * time.Second)
				continue
			}

			for _, node := range nodes {
				go func(n db.Node) {
					isAlive := storageClient.Health(context.Background(), node.Address)
					db.UpdateNodeStatus(database, n.Address, isAlive)

				}(node)
			}
			time.Sleep(10 * time.Second)
		}
	}()
}
