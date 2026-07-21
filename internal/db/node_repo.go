package db

import (
	"database/sql"
	"fmt"
	"time"
)

type Node struct {
	ID            string
	Address       string
	IsActive      bool
	LastHeartbeat time.Time
}

func RegisterNode(db *sql.DB, address string) error {
	query := `INSERT INTO nodes (address,is_active,last_heartbeat)
	          VALUES ($1,TRUE,CURRENT_TIMESTAMP)
			  ON CONFLICT (address)
			  DO UPDATE SET is_active = TRUE, last_heartbeat = CURRENT_TIMESTAMP
	`

	_, err := db.Exec(query, address)
	if err != nil {
		return fmt.Errorf("register node: %w", err)
	}
	return nil
}
func GetAllNodes(db *sql.DB) ([]Node, error) {
	rows, err := db.Query(`SELECT id, address, is_active, last_heartbeat FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Address, &n.IsActive, &n.LastHeartbeat); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}
func UpdateNodeStatus(db *sql.DB, address string, isActive bool) error {
	_, err := db.Exec(
		`UPDATE nodes SET is_active = $1, last_heartbeat = CURRENT_TIMESTAMP WHERE address = $2`,
		isActive, address,
	)
	return err
}
