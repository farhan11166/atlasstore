package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/farhan/atlasstore/pkg/pb"
)

type NodeHandler struct {
	pb.UnimplementedStorageNodeServer
	DataDir string
}

func (h *NodeHandler) SaveChunk(ctx context.Context, req *pb.SaveChunkRequest) (*pb.SaveChunkResponse, error) {
	if req.Hash == "" {
		return nil, fmt.Errorf("hash required")
	}
	if err := os.MkdirAll(h.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage dir: %w", err)
	}

	file, err := os.Create(filepath.Join(h.DataDir, req.Hash))
	if err != nil {
		return nil, fmt.Errorf("failed to create chunk file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(req.Data); err != nil {
		return nil, fmt.Errorf("failed to write chunk: %w", err)
	}
	return &pb.SaveChunkResponse{Success: true}, nil
}

func (h *NodeHandler) GetChunk(ctx context.Context, req *pb.GetChunkRequest) (*pb.GetChunkResponse, error) {
	data, err := os.ReadFile(filepath.Join(h.DataDir, req.Hash))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("chunk not found")
		}
		return nil, fmt.Errorf("failed to read chunk: %w", err)
	}

	return &pb.GetChunkResponse{Data: data}, nil
}

func (h *NodeHandler) DeleteChunk(ctx context.Context, req *pb.DeleteChunkRequest) (*pb.DeleteChunkResponse, error) {
	err := os.Remove(filepath.Join(h.DataDir, req.Hash))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("chunk not found")
		}
		return nil, fmt.Errorf("failed to delete chunk: %w", err)
	}

	return &pb.DeleteChunkResponse{Success: true}, nil
}

// Health handles GET /health
// Gateway uses this to check if the node is alive before sending chunks.
func (h *NodeHandler) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Status: "ok", DataDir: h.DataDir}, nil
}