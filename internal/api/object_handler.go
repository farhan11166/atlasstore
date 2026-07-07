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

	"github.com/farhan/atlasstore/internal/auth"
	"github.com/farhan/atlasstore/internal/db"
)

type ObjectHandler struct {
	DB            *sql.DB
	StorageClient *StorageClient
	ChunkSizeMB   int
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
	buf := make([]byte, chunkSize)

	type chunkMeta struct {
		hash string
		size int64
	}

	var metas []chunkMeta
	var totalSize int64

	for {
		n, err := io.ReadFull(r.Body, buf)

		if n > 0 {
			chunk := buf[:n]
			hash := sha256hex(chunk)
			if saveErr := h.StorageClient.SaveChunk(hash, chunk); saveErr != nil {
				http.Error(w, "failed to store chunk", http.StatusInternalServerError)
				return
			}
			metas = append(metas, chunkMeta{hash, int64(n)})
			totalSize += int64(n)
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			http.Error(w, "failed to read upload body", http.StatusInternalServerError)
			return
		}
	}

	// ← loop done. now save metadata to DB once.
	objectID, err := db.CreateObject(h.DB, userID, filename, contentType, totalSize)
	if err != nil {
		http.Error(w, "failed to save object metadata", http.StatusInternalServerError)
		return
	}
	for i, m := range metas {
		if err := db.CreateChunk(h.DB, objectID, i, m.hash, m.size, h.StorageClient.NodeAddress); err != nil {
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
		data, err := h.StorageClient.GetChunk(chunk.Hash)
		if err != nil {
			http.Error(w, "failed to retrieve chunk", http.StatusInternalServerError)
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
	for _, chunk := range chunks {
		_ = h.StorageClient.DeleteChunk(chunk.Hash)
	}
	w.WriteHeader(http.StatusNoContent)
}

func sha256hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
