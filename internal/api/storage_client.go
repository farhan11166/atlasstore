package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// for talking to node using hhtp
type StorageClient struct {
	NodeAddress string

	HTTPClient *http.Client // htpp client can be reused with connection pooling
}

func NewStorageClient(nodeAddress string) *StorageClient {
	return &StorageClient{
		NodeAddress: nodeAddress,
		HTTPClient:  &http.Client{},
	}
}

func (c *StorageClient) SaveChunk(hash string, data []byte) error {
	req, err := http.NewRequest("POST", c.NodeAddress+"/chunk", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("X-Chunk-Hash", hash)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("send chunk %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("storage node returned %d", resp.StatusCode)

	}
	return nil

}

// gets a chunk
func (c *StorageClient) GetChunk(hash string) ([]byte, error) {
	resp, err := c.HTTPClient.Get(c.NodeAddress + "/chunk/" + hash)
	if err != nil {
		return nil, fmt.Errorf("get Chunk: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("storage node returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chunk body: %w", err)
	}
	return data, nil
}

// DeleteChunk tells the storage node to remove a chunk.
func (c *StorageClient) DeleteChunk(hash string) error {
	req, err := http.NewRequest("DELETE", c.NodeAddress+"/chunk/"+hash, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete chunk: %w", err)
	}
	defer resp.Body.Close()
	return nil
}
