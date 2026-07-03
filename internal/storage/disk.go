package storage

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type NodeHandler struct {
	DataDir string
}

func (h *NodeHandler) SaveChunk(w http.ResponseWriter, r *http.Request) {
	hash := r.Header.Get("X-Chunk-Hash")
	if hash == "" {
		http.Error(w, "X-Chunk-Hash header required", http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(h.DataDir, 0755); err != nil {
		http.Error(w, "failed to create storage dir", http.StatusInternalServerError)
		return
	}
	// Filename = the hash itself
	file, err := os.Create(filepath.Join(h.DataDir, hash))
	if err != nil {
		http.Error(w, "failed to create chunk file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	if _, err := io.Copy(file, r.Body); err != nil {
		http.Error(w, "failed to write chunk", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"hash":"%s"}`, hash)
}

func (h *NodeHandler) GetChunk(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	file, err := os.Open(filepath.Join(h.DataDir, hash))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "chunk not found", http.StatusNotFound)
			return

		}
		http.Error(w, "failed to open chunk", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, file) // stream file bytes directly to response

}
func (h *NodeHandler) DeleteChunk(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	err := os.Remove(filepath.Join(h.DataDir, hash))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "chunk not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to delete chunk", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent) // 204 — success, no body
}

// Health handles GET /health
// Gateway uses this to check if the node is alive before sending chunks.
func (h *NodeHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","data_dir":"%s"}`, h.DataDir)
}
