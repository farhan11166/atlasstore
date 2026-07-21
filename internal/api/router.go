package api

import (
	"database/sql"
	"net/http"

	"github.com/farhan/atlasstore/internal/auth"
	"github.com/farhan/atlasstore/internal/config"
)

func NewRouter(cfg *config.Config, database *sql.DB) http.Handler {
	mux := http.NewServeMux()

	authHandler := &auth.Handler{
		DB:        database,
		JWTSecret: cfg.JWTSecret,
		JWTExpiry: cfg.JWTExpiryHours,
	}
	storageClient := NewStorageClient("http://localhost:" + cfg.StorageNodePort)

	objectHandler := &ObjectHandler{
		DB:            database,
		StorageClient: storageClient,
		ChunkSizeMB:   cfg.ChunkSizeMB,
	}
	nodeHandler := &NodeHandler{
		DB: database,
	}

	protected := auth.RequireAuth(cfg.JWTSecret)

	mux.HandleFunc("POST /auth/register", authHandler.Register)
	mux.HandleFunc("POST /auth/login", authHandler.Login)
	mux.Handle("/", http.FileServer(http.Dir("./web")))

	// Protected routes — Week 3, uncomment when object handlers exist
	mux.Handle("POST /objects", protected(http.HandlerFunc(objectHandler.Upload)))
	mux.Handle("GET /objects", protected(http.HandlerFunc(objectHandler.List)))
	mux.Handle("GET /objects/{id}", protected(http.HandlerFunc(objectHandler.Download)))
	mux.Handle("DELETE /objects/{id}", protected(http.HandlerFunc(objectHandler.Delete)))

	mux.Handle("POST /multipart", protected(http.HandlerFunc(objectHandler.InitMultipart)))
	mux.Handle("POST /multipart/{upload_id}/{part_number}", protected(http.HandlerFunc(objectHandler.UploadPart)))
	mux.Handle("POST /multipart/{upload_id}/complete", protected(http.HandlerFunc(objectHandler.CompleteMultipart)))

	mux.HandleFunc("POST /nodes/register", nodeHandler.Register)

	return mux

}
