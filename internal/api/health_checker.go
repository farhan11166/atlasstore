package api

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/farhan/atlasstore/internal/db"
)

func StartHealthChecker(database *sql.DB) {

	go func() {
		client := &http.Client{
			Timeout: 2 * time.Second,
		}

		for {
			nodes, err := db.GetAllNodes(database)

			if err != nil {
				log.Printf("Health check failed to get nodes: %v", err)
				time.Sleep(10 * time.Second)
				continue
			}

			for _, node := range nodes {
				resp, err := client.Get(node.Address + "/health")

				isAlive := false

				if err == nil && resp.StatusCode == http.StatusOK {
					isAlive = true
				}

				if resp != nil {
					resp.Body.Close()
				}

				if err := db.UpdateNodeStatus(database, node.Address, isAlive); err != nil {
					log.Printf("Failed to updaet status for node %s: %v", node.Address, err)
				}

			}
			time.Sleep(10 * time.Second)
		}

	}()

}
