package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct{
	GatewayPort string
    StorageNodePort string
    DBHost string
    DBPort string
    DBUser string
    DBPassword string
	DBName            string
	DBDSN             string // assembled connection string
	JWTSecret         string
	JWTExpiryHours    int
	ChunkSizeMB       int
	ReplicationFactor int					 		
}

func Load() (*Config,error){

	err := godotenv.Load(); 

	if err != nil{
		log.Println("No .env file found, reading from enviroment.")
	}

	cfg:= Config{

		GatewayPort:     getEnv("GATEWAY_PORT", "8080"),
		StorageNodePort: getEnv("STORAGE_NODE_PORT", "9000"),
		DBHost:          getEnv("DB_HOST", "localhost"),
		DBPort:          getEnv("DB_PORT", "5433"),
		DBUser:          getEnv("DB_USER", "atlasstore"),
		DBPassword:      getEnv("DB_PASSWORD", ""),
		DBName:          getEnv("DB_NAME", "atlasstore"),
		JWTSecret:       getEnv("JWT_SECRET", ""),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET must be set in environment")
	}
	if cfg.DBPassword == "" {
		return nil, fmt.Errorf("DB_PASSWORD must be set in environment")
	}

	
	cfg.JWTExpiryHours,err=strconv.Atoi(getEnv("JWT_EXPIRY","72"))
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_EXPIRY_HOURS: %w", err)
	}
	cfg.ChunkSizeMB, err = strconv.Atoi(getEnv("CHUNK_SIZE_MB", "5"))
	if err != nil {
		return nil, fmt.Errorf("invalid CHUNK_SIZE_MB: %w", err)
	}

	cfg.ReplicationFactor, err = strconv.Atoi(getEnv("REPLICATION_FACTOR", "3"))
	if err != nil {
		return nil, fmt.Errorf("invalid REPLICATION_FACTOR: %w", err)
	}

	// Assemble the DSN (Data Source Name) for database/sql / lib/pq
	cfg.DBDSN = fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName,
	)
	return &cfg, nil
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}