package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/farhan/atlasstore/internal/db"
)

type NodeHandler struct {
	DB *sql.DB
}

func (h *NodeHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return

	}
	if req.Address == "" {
		http.Error(w, "address is required", http.StatusBadRequest)
		return
	}
	if err := db.RegisterNode(h.DB, req.Address); err != nil {
		http.Error(w, "failed to register node", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
