package main

import (
	"log"

	"github.com/farhan/atlasstore/internal/config"
	"github.com/farhan/atlasstore/internal/db"
)

func main(){

	cfg,err:= config.Load()
	if err !=nil{
		log.Fatalf(" failed to load config %v",err)
	}
	database,err := db.Connect(cfg.DBSDN)
	if err!=nil{
		log.Fatalf("failed to connect to database: %v",err)
	}

	defer database.Close()
	log.Println("Connect to database")

	if err:= db.RunMigrations(database, "file://./migrations"); err !=nil{
	log.Fatalf("migrations failed: %v",err)}
	log.Println("Migrations applied")

	log.Printf("Gateway starting on port %s", cfg.GatewayPort)
}
