package api

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/farhan/atlasstore/internal/auth"
	"github.com/farhan/atlasstore/internal/crypto"
	"github.com/farhan/atlasstore/internal/db"
	"github.com/farhan/atlasstore/internal/metrics"
	"github.com/golang/snappy"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("atlasstore/object-handler")

type ObjectHandler struct {
	DB                *sql.DB
	StorageClient     *StorageClient
	RingManager       *RingManager
	ChunkSizeMB       int
	ReplicationFactor int
	QuorumSize        int
	EncryptionKey     []byte
}

type objectResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
}

func (h *ObjectHandler) Upload(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "object.upload")
	defer span.End()
	userID := r.Context().Value(auth.UserIDKey).(string)
	filename := r.Header.Get("X-Filename")
	if filename == "" {
		filename = "untitled"
	}
	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("object.filename", filename),
		attribute.Int("replication.factor", h.ReplicationFactor),
		attribute.Int("chunk.size.mb", h.ChunkSizeMB),
	)

	contentType := r.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = "application/octet-stream"
	}

	chunkSize := h.ChunkSizeMB * 1024 * 1024
	///buf := make([]byte, chunkSize) not of any use because using go routines would rewrite as per each differet thread

	type chunkMeta struct {
		index int // index added cuz of multiple threadsss
		hash  string
		size  int64
		nodes []string
	}

	var metas []chunkMeta
	var totalSize int64
	var mu sync.Mutex

	g, _ := errgroup.WithContext(ctx)
	chunkIndex := 0

	for {
		buf := make([]byte, chunkSize)
		n, err := io.ReadFull(r.Body, buf)

		if n > 0 {
			chunk := buf[:n]
			idx := chunkIndex

			g.Go(func() error {
				chunkCtx, chunkSpan := tracer.Start(ctx, "chunk.process")
				defer chunkSpan.End()
				hash := sha256hex(chunk)
				chunkSpan.SetAttributes(
					attribute.Int("chunk.index", idx),
					attribute.String("chunk.hash", hash),
					attribute.Int("chunk.size_bytes", len(chunk)),
				)

				compressedChunk := snappy.Encode(nil, chunk)
				encryptedChunk, err := crypto.Encrypt(compressedChunk, h.EncryptionKey)
				if err != nil {
					chunkSpan.RecordError(err)
					chunkSpan.SetStatus(codes.Error, "encrypt chunk failed")

					return fmt.Errorf("failed to encrypt chunk: %w", err)
				}

				nodeAddresses := h.RingManager.Ring.GetNodes(hash, h.ReplicationFactor)
				if len(nodeAddresses) == 0 {
					err := fmt.Errorf("no healthy storage node available")
					chunkSpan.RecordError(err)
					chunkSpan.SetStatus(codes.Error, "no healthy storage node available")
					return err
				}

				successNodes, saveErrs := h.StorageClient.SaveChunk(chunkCtx, nodeAddresses, hash, encryptedChunk)
				quorum := (h.ReplicationFactor / 2) + 1
				if len(successNodes) < quorum {
					fmt.Printf("SaveChunk errors in Upload: %v\n", saveErrs)
					err := fmt.Errorf("upload failed: quorum not met (needed %d, got %d). errors: %v", quorum, len(successNodes), saveErrs)
					chunkSpan.RecordError(err)
					chunkSpan.SetStatus(codes.Error, "quorum not met")
					return err
				}
				mu.Lock()
				metas = append(metas, chunkMeta{index: idx, hash: hash, size: int64(n), nodes: successNodes})
				totalSize += int64(n)
				mu.Unlock()
				chunkSpan.SetAttributes(attribute.Int("chunk.saved_replicas", len(successNodes)))

				return nil

			})

			chunkIndex++
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			http.Error(w, "failed to read upload body", http.StatusInternalServerError)
			return
		}
	}

	if err := g.Wait(); err != nil {
		http.Error(w, "failed to store chunk concurrently", http.StatusInternalServerError)
		return
	}

	// ← loop done. now save metadata to DB once.
	objectID, err := db.CreateObject(h.DB, userID, filename, contentType, totalSize)
	if err != nil {
		http.Error(w, "failed to save object metadata", http.StatusInternalServerError)
		return
	}
	for _, m := range metas {
		if err := db.CreateChunk(h.DB, objectID, m.index, m.hash, m.size, m.nodes); err != nil {
			http.Error(w, "failed to save chunk metadata", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(objectResponse{
		ID: objectID, Name: filename,
		SizeBytes: totalSize, ContentType: contentType,
	})
	metrics.UploadsTotal.Inc()

}

func (h *ObjectHandler) Download(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "object.download")
	defer span.End()

	userID := r.Context().Value(auth.UserIDKey).(string)
	objectID := r.PathValue("id") // to retrive id
	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("object.id", objectID),
	)

	obj, err := db.GetObjectByID(h.DB, objectID, userID)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get object metadata failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return

	}

	if obj == nil {
		span.SetStatus(codes.Error, "object not found")
		http.Error(w, "object not found", http.StatusNotFound)
		return
	}
	chunks, err := db.GetChunksByObjectID(h.DB, objectID)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get chunk metadata failed")
		http.Error(w, "failed to fetch chunk metadata", http.StatusInternalServerError)
		return
	}

	span.SetAttributes(attribute.Int("object.chunk_count", len(chunks)))

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, obj.Name))

	for _, chunk := range chunks {
		chunkCtx, chunkSpan := tracer.Start(ctx, "chunk.fetch")
		chunkSpan.SetAttributes(
			attribute.String("chunk.hash", chunk.Hash),
			attribute.Int("chunk.replicas", len(chunk.NodeAddresses)),
		)

		var data []byte
		var err error
		var success bool

		for _, nodeAddr := range chunk.NodeAddresses {
			replicaCtx, replicaSpan := tracer.Start(chunkCtx, "chunk.fetch.replica")
			replicaSpan.SetAttributes(attribute.String("node.address", nodeAddr))

			data, err = h.StorageClient.GetChunk(replicaCtx, nodeAddr, chunk.Hash)
			if err != nil {
				replicaSpan.RecordError(err)
				replicaSpan.SetStatus(codes.Error, "replica fetch failed")
				replicaSpan.End()
				fmt.Printf("Warning: Failed to retrieve chunk %s from %s: %v\n", chunk.Hash, nodeAddr, err)
				continue
			}
			replicaSpan.End()

			decryptedData, err := crypto.Decrypt(data, h.EncryptionKey)
			if err != nil {
				chunkSpan.RecordError(err)
				fmt.Printf("Warning: Failed to decrypt chunk %s: %v\n", chunk.Hash, err)
				continue
			}

			decompressedData, err := snappy.Decode(nil, decryptedData)
			if err != nil {
				chunkSpan.RecordError(err)
				fmt.Printf("Warning: Failed to decompress chunk %s: %v\n", chunk.Hash, err)
				continue
			}

			if actualHash := sha256hex(decompressedData); actualHash != chunk.Hash {
				err = fmt.Errorf("data corruption on node %s for chunk %s", nodeAddr, chunk.Hash)
				chunkSpan.RecordError(err)
				fmt.Printf("Warning: Data corruption on node %s for chunk %s\n", nodeAddr, chunk.Hash)
				continue
			}

			data = decompressedData
			chunkSpan.SetAttributes(
				attribute.String("chunk.source_node", nodeAddr),
				attribute.Int("chunk.size_bytes", len(data)),
			)
			success = true
			break
		}

		if !success {
			err := fmt.Errorf("all replicas failed or corrupted")
			chunkSpan.RecordError(err)
			chunkSpan.SetStatus(codes.Error, "replica exhaustion")
			chunkSpan.End()
			span.RecordError(err)
			span.SetStatus(codes.Error, "download failed")
			http.Error(w, "all replicas failed or corrupted", http.StatusInternalServerError)
			return
		}

		if _, err := w.Write(data); err != nil {
			chunkSpan.RecordError(err)
			chunkSpan.SetStatus(codes.Error, "response write failed")
			chunkSpan.End()
			span.RecordError(err)
			span.SetStatus(codes.Error, "download response write failed")
			return
		}
		chunkSpan.End()
	}
	metrics.DownloadsTotal.Inc()
}

func (h *ObjectHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserIDKey).(string)

	objects, err := db.ListObjects(h.DB, userID)

	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	resp := make([]objectResponse, 0, len(objects)) // never return null
	for _, o := range objects {
		resp = append(resp, objectResponse{
			ID: o.ID, Name: o.Name,
			SizeBytes: o.SizeBytes, ContentType: o.ContentType,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

}

func (h *ObjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	objectID := r.PathValue("id")
	userID := r.Context().Value(auth.UserIDKey).(string)

	chunks, err := db.GetChunksByObjectID(h.DB, objectID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	// Delete from PostgreSQL — CASCADE removes chunks table rows too
	if err := db.DeleteObject(h.DB, objectID, userID); err != nil {
		http.Error(w, "failed to delete object", http.StatusInternalServerError)
		return
	}
	// Delete from storage node — best effort, don't fail if node is down
	//phase 2 word
	var wg sync.WaitGroup
	for _, chunk := range chunks {
		for _, nodeAddr := range chunk.NodeAddresses {
			wg.Add(1)
			go func(hash string, addr string) {
				defer wg.Done()
				_ = h.StorageClient.DeleteChunk(r.Context(), addr, hash)
			}(chunk.Hash, nodeAddr)
		}
	}

	wg.Wait() // here i wait for all the deleting to finish or else it might not finish but header might be wrriten
	w.WriteHeader(http.StatusNoContent)
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (h *ObjectHandler) InitMultipart(w http.ResponseWriter, r *http.Request) {
	_, span := tracer.Start(r.Context(), "multipart.init")
	defer span.End()

	userID := r.Context().Value(auth.UserIDKey).(string)
	span.SetAttributes(attribute.String("user.id", userID))

	var req struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid request body")
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	uploadID, err := db.CreateMultipartUpload(h.DB, userID, req.Filename, req.ContentType)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create multipart upload failed")
		fmt.Println("DB ERROR:", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	span.SetAttributes(
		attribute.String("upload.id", uploadID),
		attribute.String("object.filename", req.Filename),
	)

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{"upload_id": uploadID})
}
func (h *ObjectHandler) UploadPart(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "multipart.upload_part")
	defer span.End()

	uploadID := r.PathValue("upload_id")
	partStr := r.PathValue("part_number")
	span.SetAttributes(
		attribute.String("upload.id", uploadID),
		attribute.String("part.number", partStr),
	)

	var partNumber int
	fmt.Sscanf(partStr, "%d", &partNumber)

	chunkData, err := io.ReadAll(io.LimitReader(r.Body, int64(h.ChunkSizeMB*1024*1024+1024)))
	if err != nil || len(chunkData) == 0 {
		if err == nil {
			err = fmt.Errorf("empty part body")
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "read part failed")
		http.Error(w, "failed to read part", http.StatusBadRequest)
		return
	}

	hash := sha256hex(chunkData)
	span.SetAttributes(
		attribute.String("chunk.hash", hash),
		attribute.Int("chunk.size_bytes", len(chunkData)),
	)

	compressedChunk := snappy.Encode(nil, chunkData)
	encryptedChunk, err := crypto.Encrypt(compressedChunk, h.EncryptionKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "encrypt part failed")
		http.Error(w, fmt.Sprintf("Failed to encrypt chunk: %v", err), http.StatusInternalServerError)
		return
	}

	nodeAddresses := h.RingManager.Ring.GetNodes(hash, h.ReplicationFactor)
	if len(nodeAddresses) == 0 {
		err := fmt.Errorf("no healthy storage node available")
		span.RecordError(err)
		span.SetStatus(codes.Error, "no healthy storage node available")
		http.Error(w, "no healthy storage node available", http.StatusServiceUnavailable)
		return
	}

	successNodes, saveErrs := h.StorageClient.SaveChunk(ctx, nodeAddresses, hash, encryptedChunk)
	quorum := (h.ReplicationFactor / 2) + 1
	if len(successNodes) < quorum {
		fmt.Printf("SaveChunk errors in UploadPart: %v\n", saveErrs)
		err := fmt.Errorf("multipart save quorum not met: needed %d, got %d", quorum, len(successNodes))
		span.RecordError(err)
		span.SetStatus(codes.Error, "quorum not met")
		http.Error(w, "failed to store chunk on storage node (quorum not met)", http.StatusInternalServerError)
		return
	}
	span.SetAttributes(attribute.Int("chunk.saved_replicas", len(successNodes)))

	if err := db.CreateMultipartChunk(h.DB, uploadID, partNumber, hash, int64(len(chunkData)), successNodes); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "save multipart metadata failed")
		http.Error(w, "failed to save chunk metadata", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *ObjectHandler) CompleteMultipart(w http.ResponseWriter, r *http.Request) {
	_, span := tracer.Start(r.Context(), "multipart.complete")
	defer span.End()

	uploadID := r.PathValue("upload_id")
	userID := r.Context().Value(auth.UserIDKey).(string)
	span.SetAttributes(
		attribute.String("upload.id", uploadID),
		attribute.String("user.id", userID),
	)
	objectID, err := db.CompleteMultipartUpload(h.DB, uploadID, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "complete multipart upload failed")
		http.Error(w, "failed to complete upload", http.StatusInternalServerError)
		return
	}
	span.SetAttributes(attribute.String("object.id", objectID))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"object_id": objectID})
}
