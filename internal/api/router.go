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

	mux.HandleFunc("POST /auth/register", authHandler.Register)
	mux.HandleFunc("POST /auth/login", authHandler.Login)

	// Protected routes — Week 3, uncomment when object handlers exist
	// protected := auth.RequireAuth(cfg.JWTSecret)
	// mux.Handle("POST /objects",       protected(http.HandlerFunc(objectHandler.Upload)))
	// mux.Handle("GET /objects",        protected(http.HandlerFunc(objectHandler.List)))
	// mux.Handle("GET /objects/{id}",   protected(http.HandlerFunc(objectHandler.Download)))
	// mux.Handle("DELETE /objects/{id}", protected(http.HandlerFunc(objectHandler.Delete)))
	return mux

}
