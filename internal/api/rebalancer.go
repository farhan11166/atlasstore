package api

import (
	"context"
	"log"
	"time"

	"github.com/farhan/atlasstore/internal/db"
)

func StartRebalancer(ringManager *RingManager, storageClient *StorageClient, replicationFactor int) {
	go func() {
		for {
			// Run rebalancer every 60 seconds
			time.Sleep(60 * time.Second)

			chunks, err := db.GetAllChunks(ringManager.DB)
			if err != nil {
				log.Printf("Rebalancer: failed to get chunks: %v", err)
				continue
			}

			if len(chunks) == 0 {
				continue
			}

			ringManager.mu.RLock()
			ringIsEmpty := ringManager.Ring.IsEmpty()
			ringManager.mu.RUnlock()

			if ringIsEmpty {
				continue
			}

			for _, chunk := range chunks {
				ctx := context.Background()

				expectedNodes := ringManager.Ring.GetNodes(chunk.Hash, replicationFactor)

				// Create maps for easy lookup
				expectedMap := make(map[string]bool)
				for _, n := range expectedNodes {
					expectedMap[n] = true
				}

				currentMap := make(map[string]bool)
				for _, n := range chunk.NodeAddresses {
					currentMap[n] = true
				}

				// Find missing nodes (where chunk should be but isn't)
				var missingNodes []string
				for n := range expectedMap {
					if !currentMap[n] {
						missingNodes = append(missingNodes, n)
					}
				}

				// Find obsolete nodes (where chunk is but shouldn't be)
				var obsoleteNodes []string
				for n := range currentMap {
					if !expectedMap[n] {
						obsoleteNodes = append(obsoleteNodes, n)
					}
				}

				if len(missingNodes) == 0 && len(obsoleteNodes) == 0 {
					continue // Chunk is perfectly balanced
				}

				log.Printf("Rebalancing chunk %s. Missing: %v, Obsolete: %v", chunk.Hash, missingNodes, obsoleteNodes)

				// Download chunk from a current node
				var chunkData []byte
				var getErr error
				for n := range currentMap {
					chunkData, getErr = storageClient.GetChunk(ctx, n, chunk.Hash)
					if getErr == nil {
						break
					}
				}

				if getErr != nil || len(chunkData) == 0 {
					log.Printf("Rebalancer: failed to retrieve chunk %s to move", chunk.Hash)
					continue
				}

				// Upload to missing nodes
				if len(missingNodes) > 0 {
					successNodes, errs := storageClient.SaveChunk(ctx, missingNodes, chunk.Hash, chunkData)
					if len(errs) > 0 {
						log.Printf("Rebalancer: encountered errors saving chunk %s to some new nodes: %v", chunk.Hash, errs)
					}
					// Update DB
					for _, n := range successNodes {
						db.AddChunkLocation(ringManager.DB, chunk.ID, n)
					}
				}

				// Delete from obsolete nodes
				for _, n := range obsoleteNodes {
					err := storageClient.DeleteChunk(ctx, n, chunk.Hash)
					if err != nil {
						log.Printf("Rebalancer: failed to delete chunk %s from %s", chunk.Hash, n)
					}
					db.RemoveChunkLocation(ringManager.DB, chunk.ID, n)
				}
			}
		}
	}()
}
