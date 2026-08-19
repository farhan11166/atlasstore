package main

import (
	"context"
	"log"
	"net/http"

	"github.com/farhan/atlasstore/internal/api"
	"github.com/farhan/atlasstore/internal/config"
	"github.com/farhan/atlasstore/internal/db"
	"github.com/farhan/atlasstore/internal/telemetry"
)

func main() {
	ctx := context.Background()

	shutdownTracing, err := telemetry.Init(ctx, "atlasstore-gateway")
	if err != nil {
		log.Fatalf("failed to init tracing: %v", err)
	}
	defer shutdownTracing(context.Background())

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf(" failed to load config %v", err)
	}
	database, err := db.Connect(cfg.DBDSN)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	defer database.Close()
	log.Println("Connect to database")

	if err := db.RunMigrations(database, "file://./migrations"); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}
	log.Println("Migrations applied")
	router := api.NewRouter(cfg, database)
	router = telemetry.HTTP("gateway-http", router)
	addr := ":" + cfg.GatewayPort
	log.Printf("Gateway listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}

}
