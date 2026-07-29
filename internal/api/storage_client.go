package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/farhan/atlasstore/pkg/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type StorageClient struct {
	conns   map[string]*grpc.ClientConn
	connsMu sync.RWMutex
}

func NewStorageClient() *StorageClient {
	return &StorageClient{
		conns: make(map[string]*grpc.ClientConn),
	}
}

// getClient retrieves a cached connection or dials a new one
func (c *StorageClient) getClient(address string) (pb.StorageNodeClient, error) {
	c.connsMu.RLock()
	conn, ok := c.conns[address]
	c.connsMu.RUnlock()

	if ok {
		return pb.NewStorageNodeClient(conn), nil
	}

	c.connsMu.Lock()
	defer c.connsMu.Unlock()

	if conn, ok := c.conns[address]; ok {
		return pb.NewStorageNodeClient(conn), nil
	}

	// Connect via gRPC
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	c.conns[address] = conn
	return pb.NewStorageNodeClient(conn), nil
}

func (c *StorageClient) SaveChunk(nodeAddresses []string, hash string, data []byte) []error {
	var errs []error
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, addr := range nodeAddresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			client, err := c.getClient(address)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("dial %s: %w", address, err))
				mu.Unlock()
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err = client.SaveChunk(ctx, &pb.SaveChunkRequest{Hash: hash, Data: data})
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("save chunk to %s: %w", address, err))
				mu.Unlock()
			}
		}(addr)
	}
	wg.Wait()
	return errs
}

func (c *StorageClient) GetChunk(nodeAddress string, hash string) ([]byte, error) {
	client, err := c.getClient(nodeAddress)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.GetChunk(ctx, &pb.GetChunkRequest{Hash: hash})
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func (c *StorageClient) DeleteChunk(nodeAddress string, hash string) error {
	client, err := c.getClient(nodeAddress)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.DeleteChunk(ctx, &pb.DeleteChunkRequest{Hash: hash})
	return err
}

// Health is a helper to check if a node is alive via gRPC
func (c *StorageClient) Health(nodeAddress string) bool {
	client, err := c.getClient(nodeAddress)
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.Health(ctx, &pb.HealthRequest{})
	return err == nil
}
