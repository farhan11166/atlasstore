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
)

type ObjectHandler struct {
	DB                *sql.DB
	StorageClient     *StorageClient
	ChunkSizeMB       int
	ReplicationFactor int
	EncryptionKey     []byte
}

type objectResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
}

func (h *ObjectHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserIDKey).(string)
	filename := r.Header.Get("X-Filename")
	if filename == "" {
		filename = "untitled"
	}

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

	g, _ := errgroup.WithContext(r.Context())
	chunkIndex := 0

	for {
		buf := make([]byte, chunkSize)
		n, err := io.ReadFull(r.Body, buf)

		if n > 0 {
			chunk := buf[:n]
			idx := chunkIndex

			g.Go(func() error {
				hash := sha256hex(chunk)

				encryptedChunk, err := crypto.Encrypt(chunk, h.EncryptionKey)
				if err != nil {
					return fmt.Errorf("failed to encrypt chunk: %w", err)
				}

				nodeAddresses, err := GetHealthyNodes(h.DB, h.ReplicationFactor)
				if err != nil {
					return err
				}

				if saveErrs := h.StorageClient.SaveChunk(nodeAddresses, hash, encryptedChunk); len(saveErrs) > 0 {
					fmt.Printf("SaveChunk errors in Upload: %v\n", saveErrs)
					return fmt.Errorf("upload failed: %v", saveErrs)
				}
				mu.Lock()
				metas = append(metas, chunkMeta{index: idx, hash: hash, size: int64(n), nodes: nodeAddresses})
				totalSize += int64(n)
				mu.Unlock()

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

}

func (h *ObjectHandler) Download(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserIDKey).(string)
	objectID := r.PathValue("id") // to retrive id
	obj, err := db.GetObjectByID(h.DB, objectID, userID)

	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return

	}

	if obj == nil {
		http.Error(w, "object not found", http.StatusNotFound)
		return
	}
	chunks, err := db.GetChunksByObjectID(h.DB, objectID)

	if err != nil {
		http.Error(w, "failed to fetch chunk metadata", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, obj.Name))

	for _, chunk := range chunks {
		var data []byte
		var err error
		var success bool

		for _, nodeAddr := range chunk.NodeAddresses {
			data, err = h.StorageClient.GetChunk(nodeAddr, chunk.Hash)
			if err != nil {
				fmt.Printf("Warning: Failed to retrieve chunk %s from %s: %v\n", chunk.Hash, nodeAddr, err)
				continue
			}

			decryptedData, err := crypto.Decrypt(data, h.EncryptionKey)
			if err != nil {
				fmt.Printf("Warning: Failed to decrypt chunk %s: %v\n", chunk.Hash, err)
				continue
			}

			if actualHash := sha256hex(decryptedData); actualHash != chunk.Hash {
				fmt.Printf("Warning: Data corruption on node %s for chunk %s\n", nodeAddr, chunk.Hash)
				continue
			}
			
			data = decryptedData
			success = true
			break
		}

		if !success {
			http.Error(w, "all replicas failed or corrupted", http.StatusInternalServerError)
			return
		}

		if _, err := w.Write(data); err != nil {
			return
		}
	}
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
				_ = h.StorageClient.DeleteChunk(addr, hash)
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
	userID := r.Context().Value(auth.UserIDKey).(string)

	var req struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	uploadID, err := db.CreateMultipartUpload(h.DB, userID, req.Filename, req.ContentType)

	if err != nil {
		fmt.Println("DB ERROR:", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{"upload_id": uploadID})
}
func (h *ObjectHandler) UploadPart(w http.ResponseWriter, r *http.Request) {

	uploadID := r.PathValue("upload_id")
	partStr := r.PathValue("part_number")

	var partNumber int
	fmt.Sscanf(partStr, "%d", &partNumber)

	chunkData, err := io.ReadAll(io.LimitReader(r.Body, int64(h.ChunkSizeMB*1024*1024+1024)))
	if err != nil || len(chunkData) == 0 {
		http.Error(w, "failed to read part", http.StatusBadRequest)
		return
	}

	hash := sha256hex(chunkData)

	encryptedChunk, err := crypto.Encrypt(chunkData, h.EncryptionKey)
	if err != nil {
		http.Error(w, "failed to encrypt chunk", http.StatusInternalServerError)
		return
	}

	nodeAddresses, err := GetHealthyNodes(h.DB, h.ReplicationFactor)
	if err != nil {
		http.Error(w, "no healthy storage node available", http.StatusServiceUnavailable)
		return
	}

	if saveErrs := h.StorageClient.SaveChunk(nodeAddresses, hash, encryptedChunk); len(saveErrs) > 0 {
		fmt.Printf("SaveChunk errors in UploadPart: %v\n", saveErrs)
		http.Error(w, "failed to store chunk on storage node", http.StatusInternalServerError)
		return
	}

	if err := db.CreateMultipartChunk(h.DB, uploadID, partNumber, hash, int64(len(chunkData)), nodeAddresses); err != nil {
		http.Error(w, "failed to save chunk metadata", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *ObjectHandler) CompleteMultipart(w http.ResponseWriter, r *http.Request) {
	uploadID := r.PathValue("upload_id")
	userID := r.Context().Value(auth.UserIDKey).(string)
	objectID, err := db.CompleteMultipartUpload(h.DB, uploadID, userID)
	if err != nil {
		http.Error(w, "failed to complete upload", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"object_id": objectID})
}
func GetHealthyNodes(db *sql.DB, count int) ([]string, error) {
	rows, err := db.Query(`SELECT address FROM nodes WHERE is_active = TRUE ORDER BY RANDOM() LIMIT $1`, count)
	if err != nil {
		return nil, fmt.Errorf("no healthy nodes available: %w", err)
	}
	defer rows.Close()
	var nodes []string
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			return nil, err
		}
		nodes = append(nodes, addr)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no healthy nodes available")
	}
	return nodes, nil
}
