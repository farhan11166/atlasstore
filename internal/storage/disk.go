package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/farhan/atlasstore/pkg/pb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("atlasstore/storage-node")

type NodeHandler struct {
	pb.UnimplementedStorageNodeServer
	DataDir string
}

func (h *NodeHandler) SaveChunk(ctx context.Context, req *pb.SaveChunkRequest) (*pb.SaveChunkResponse, error) {
	_, span := tracer.Start(ctx, "storage.SaveChunk")
	defer span.End()

	span.SetAttributes(
		attribute.String("chunk.hash", req.Hash),
		attribute.Int("chunk.size_bytes", len(req.Data)),
		attribute.String("storage.data_dir", h.DataDir),
	)

	if req.Hash == "" {
		err := fmt.Errorf("hash required")
		span.RecordError(err)
		span.SetStatus(codes.Error, "missing hash")
		return nil, err
	}
	if err := os.MkdirAll(h.DataDir, 0755); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "mkdir failed")
		return nil, fmt.Errorf("failed to create storage dir: %w", err)
	}

	file, err := os.Create(filepath.Join(h.DataDir, req.Hash))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create file failed")
		return nil, fmt.Errorf("failed to create chunk file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(req.Data); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "write failed")
		return nil, fmt.Errorf("failed to write chunk: %w", err)
	}
	return &pb.SaveChunkResponse{Success: true}, nil
}

func (h *NodeHandler) GetChunk(ctx context.Context, req *pb.GetChunkRequest) (*pb.GetChunkResponse, error) {
	_, span := tracer.Start(ctx, "storage.GetChunk")
	defer span.End()

	span.SetAttributes(
		attribute.String("chunk.hash", req.Hash),
		attribute.String("storage.data_dir", h.DataDir),
	)

	data, err := os.ReadFile(filepath.Join(h.DataDir, req.Hash))
	if err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("chunk not found")
			span.RecordError(err)
			span.SetStatus(codes.Error, "chunk not found")
			return nil, err
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "read failed")
		return nil, fmt.Errorf("failed to read chunk: %w", err)
	}

	span.SetAttributes(attribute.Int("chunk.size_bytes", len(data)))
	return &pb.GetChunkResponse{Data: data}, nil
}

func (h *NodeHandler) DeleteChunk(ctx context.Context, req *pb.DeleteChunkRequest) (*pb.DeleteChunkResponse, error) {
	_, span := tracer.Start(ctx, "storage.DeleteChunk")
	defer span.End()

	span.SetAttributes(
		attribute.String("chunk.hash", req.Hash),
		attribute.String("storage.data_dir", h.DataDir),
	)

	err := os.Remove(filepath.Join(h.DataDir, req.Hash))
	if err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("chunk not found")
			span.RecordError(err)
			span.SetStatus(codes.Error, "chunk not found")
			return nil, err
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "delete failed")
		return nil, fmt.Errorf("failed to delete chunk: %w", err)
	}

	return &pb.DeleteChunkResponse{Success: true}, nil
}

// Health handles GET /health
// Gateway uses this to check if the node is alive before sending chunks.
func (h *NodeHandler) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	_, span := tracer.Start(ctx, "storage.Health")
	defer span.End()

	span.SetAttributes(attribute.String("storage.data_dir", h.DataDir))
	return &pb.HealthResponse{Status: "ok", DataDir: h.DataDir}, nil
}
