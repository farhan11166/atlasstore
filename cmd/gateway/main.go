package main

import (
	"log"
	"net/http"

	"github.com/farhan/atlasstore/internal/api"
	"github.com/farhan/atlasstore/internal/config"
	"github.com/farhan/atlasstore/internal/db"
)

func main() {

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
	addr := ":" + cfg.GatewayPort
	log.Printf("Gateway listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}

}
