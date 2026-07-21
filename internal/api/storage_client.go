package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// for talking to node using hhtp
type StorageClient struct {
	///NodeAddress string not needed as

	HTTPClient *http.Client // htpp client can be reused with connection pooling
}

func NewStorageClient(nodeAddress string) *StorageClient {
	return &StorageClient{
		//NodeAddress: nodeAddress, not needed changing the code as eper the new nodes
		HTTPClient: &http.Client{},
	}
}

func (c *StorageClient) SaveChunk(nodeAddress string, hash string, data []byte) error {
	req, err := http.NewRequest("POST", nodeAddress+"/chunk", bytes.NewReader(data))
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
func (c *StorageClient) GetChunk(nodeAddress string, hash string) ([]byte, error) {
	resp, err := c.HTTPClient.Get(nodeAddress + "/chunk/" + hash)
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
func (c *StorageClient) DeleteChunk(nodeAddress string, hash string) error {
	req, err := http.NewRequest("DELETE", nodeAddress+"/chunk/"+hash, nil)
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
