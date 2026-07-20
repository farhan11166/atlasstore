package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Object struct {
	ID          string
	UserID      string
	Name        string
	SizeBytes   int64
	ContentType string
	CreatedAt   time.Time
}

type Chunk struct {
	ID          string
	ObjectID    string
	ChunkIndex  int
	Hash        string
	SizeBytes   int64
	NodeAddress string
}

func CreateObject(db *sql.DB, userID, name, contentType string, sizeBytes int64) (string, error) {
	var id string
	query := `INSERT INTO objects (user_id, name, content_type, size_bytes)
		VALUES ($1, $2, $3, $4)
		RETURNING id`
	err := db.QueryRow(query, userID, name, contentType, sizeBytes).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create object: %w", err)
	}
	return id, nil
}

func CreateChunk(db *sql.DB, objectID string, chunkIndex int, hash string, sizeBytes int64, nodeAddress string) error {
	query := `
		INSERT INTO chunks (object_id, chunk_index, hash, size, node_address)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := db.Exec(query, objectID, chunkIndex, hash, sizeBytes, nodeAddress)
	if err != nil {
		return fmt.Errorf("create chunk: %w", err)
	}
	return nil
}

// GetObjectByID fetches an object by ID, verifying it belongs to the given user.
// Returns nil, nil if not found or not owned by user.
func GetObjectByID(db *sql.DB, objectID, userID string) (*Object, error) {
	o := &Object{}
	query := `
		SELECT id, user_id, name, content_type, size_bytes, created_at
		FROM objects
		WHERE id = $1 AND user_id = $2`
	err := db.QueryRow(query, objectID, userID).Scan(
		&o.ID, &o.UserID, &o.Name, &o.ContentType, &o.SizeBytes, &o.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	return o, nil
}

func GetChunksByObjectID(db *sql.DB, objectID string) ([]Chunk, error) {
	query := `
		SELECT id, object_id, chunk_index, hash, size, node_address
		FROM chunks
		WHERE object_id = $1
		ORDER BY chunk_index ASC`
	rows, err := db.Query(query, objectID)
	if err != nil {
		return nil, fmt.Errorf("get chunks: %w", err)

	}
	defer rows.Close()
	var chunks []Chunk

	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.ObjectID, &c.ChunkIndex, &c.Hash, &c.SizeBytes, &c.NodeAddress); err != nil {
			return nil, fmt.Errorf("scan chunk %w", err)
		}
		chunks = append(chunks, c) //basic slice
	}

	return chunks, nil
}

func ListObjects(db *sql.DB, userID string) ([]Object, error) {
	query := `
		SELECT id, user_id, name, content_type, size_bytes, created_at
		FROM objects
		WHERE user_id = $1
		ORDER BY created_at DESC`
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}
	defer rows.Close()

	var objects []Object
	for rows.Next() {
		var o Object
		if err := rows.Scan(&o.ID, &o.UserID, &o.Name, &o.ContentType, &o.SizeBytes, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan object: %w", err)
		}
		objects = append(objects, o)
	}
	return objects, nil
}

func DeleteObject(db *sql.DB, objectID, userID string) error {
	result, err := db.Exec(
		`DELETE FROM objects WHERE id = $1 AND user_id = $2`, objectID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("object not found or owned by user")
	}
	return nil
}

func CreateMultipartUpload(db *sql.DB, userID, filename, contentType string) (string, error) {
	var id string
	err := db.QueryRow(
		`INSERT INTO multipart_uploads (user_id, filename, content_type) VALUES ($1, $2, $3) RETURNING id`,
		userID, filename, contentType,
	).Scan(&id)
	return id, err
}

func CompleteMultipartUpload(db *sql.DB, uploadID, userID string) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// 1. Get the upload details
	var filename, contentType string
	err = tx.QueryRow(`SELECT filename, content_type FROM multipart_uploads WHERE id = $1 AND user_id = $2`, uploadID, userID).Scan(&filename, &contentType)
	if err != nil {
		return "", err
	}

	// 2. Sum the total size of all uploaded chunks
	var totalSize int64
	err = tx.QueryRow(`SELECT COALESCE(SUM(size_bytes), 0) FROM multipart_chunks WHERE upload_id = $1`, uploadID).Scan(&totalSize)
	if err != nil {
		return "", err
	}

	// 3. Create the final main object (THIS IS WHAT YOU MISSED!)
	var newObjectID string
	err = tx.QueryRow(`INSERT INTO objects (user_id, name, content_type, size_bytes) VALUES ($1, $2, $3, $4) RETURNING id`, userID, filename, contentType, totalSize).Scan(&newObjectID)
	if err != nil {
		return "", err
	}

	// 4. Move all chunks over to the main chunks table
	_, err = tx.Exec(`INSERT INTO chunks (object_id, chunk_index, hash, size, node_address)
		 SELECT $1, chunk_index, hash, size_bytes, node_address FROM multipart_chunks WHERE upload_id = $2`, newObjectID, uploadID)
	if err != nil {
		return "", err
	}

	// 5. Delete the temp upload
	_, err = tx.Exec(`DELETE FROM multipart_uploads WHERE id = $1`, uploadID)
	if err != nil {
		return "", err
	}

	return newObjectID, tx.Commit()
}

func CreateMultipartChunk(db *sql.DB, uploadID string, chunkIndex int, hash string, size int64, nodeAddress string) error {
	_, err := db.Exec(
		`INSERT INTO multipart_chunks (upload_id, chunk_index, hash, size_bytes, node_address) 
		 VALUES ($1, $2, $3, $4, $5)`,
		uploadID, chunkIndex, hash, size, nodeAddress,
	)
	return err
}
