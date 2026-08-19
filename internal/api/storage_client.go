package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/farhan/atlasstore/pkg/pb"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type CircuitState string

const (
	StateClosed   CircuitState = "CLOSED"
	StateOpen     CircuitState = "OPEN"
	StateHalfOpen CircuitState = "HALF_OPEN"
)

type CircuitBreaker struct {
	state       CircuitState
	failures    int
	lastFailure time.Time
	mu          sync.Mutex
}

type StorageClient struct {
	conns   map[string]*grpc.ClientConn
	connsMu sync.RWMutex

	breakers   map[string]*CircuitBreaker
	breakersMu sync.RWMutex
}

func NewStorageClient() *StorageClient {
	return &StorageClient{
		conns:    make(map[string]*grpc.ClientConn),
		breakers: make(map[string]*CircuitBreaker),
	}
}

func (c *StorageClient) getBreaker(address string) *CircuitBreaker {
	c.breakersMu.Lock()
	defer c.breakersMu.Unlock()

	if b, exists := c.breakers[address]; exists {
		return b
	}
	//in caase doesnt existss
	b := &CircuitBreaker{state: StateClosed}
	c.breakers[address] = b
	return b

}

func (c *StorageClient) executewithBreaker(address string, operation func() error) error {
	cb := c.getBreaker((address))

	cb.mu.Lock()
	if cb.state == StateOpen {
		if time.Since(cb.lastFailure) > 30*time.Second {
			cb.state = StateHalfOpen // Time to test the waters!
		} else {
			cb.mu.Unlock()
			return fmt.Errorf("circuit breaker open for node %s", address) // INSTANT FAIL

		}

	}

	cb.mu.Unlock()

	err := operation()
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if err != nil {
		// Operation failed!
		cb.failures++
		cb.lastFailure = time.Now()
		if cb.failures >= 3 {
			cb.state = StateOpen // Trip the breaker!
		}
		return err
	}
	// Operation succeeded! Reset everything.
	cb.failures = 0
	cb.state = StateClosed
	return nil

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

	// Connect via gRPC (increase limits to 10MB)
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(10*1024*1024),
			grpc.MaxCallSendMsgSize(10*1024*1024),
		),
	)
	if err != nil {
		return nil, err
	}
	c.conns[address] = conn
	return pb.NewStorageNodeClient(conn), nil
}

func (c *StorageClient) SaveChunk(ctx context.Context, nodeAddresses []string, hash string, data []byte) ([]string, []error) {
	var errs []error
	var successAddrs []string
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, addr := range nodeAddresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()

			err := c.executewithBreaker(address, func() error {
				client, err := c.getClient(address)
				if err != nil {
					return err
				}

				ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				_, err = client.SaveChunk(ctx, &pb.SaveChunkRequest{Hash: hash, Data: data})
				return err
			})
			mu.Lock()

			if err != nil {

				errs = append(errs, fmt.Errorf("save chunk to %s: %w", address, err))

			} else {
				successAddrs = append(successAddrs, address)
			}
			mu.Unlock()
		}(addr)
	}
	wg.Wait()
	return successAddrs, errs
}

func (c *StorageClient) GetChunk(ctx context.Context, nodeAddress string, hash string) ([]byte, error) {
	var resultData []byte

	err := c.executewithBreaker(nodeAddress, func() error {
		client, err := c.getClient(nodeAddress)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		resp, err := client.GetChunk(ctx, &pb.GetChunkRequest{Hash: hash})
		if err != nil {
			return err
		}
		resultData = resp.Data
		return nil
	})

	return resultData, err
}

func (c *StorageClient) DeleteChunk(ctx context.Context, nodeAddress string, hash string) error {
	return c.executewithBreaker(nodeAddress, func() error {
		client, err := c.getClient(nodeAddress)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		_, err = client.DeleteChunk(ctx, &pb.DeleteChunkRequest{Hash: hash})
		return err
	})
}

// Health is a helper to check if a node is alive via gRPC
func (c *StorageClient) Health(ctx context.Context, nodeAddress string) bool {
	err := c.executewithBreaker(nodeAddress, func() error {
		client, err := c.getClient(nodeAddress)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		_, err = client.Health(ctx, &pb.HealthRequest{})
		return err
	})

	return err == nil
}
