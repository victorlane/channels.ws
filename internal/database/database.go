package database

import (
	"database/sql"
	"fmt"
	"time"

	"channels.ws/internal/models"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func New(databasePath string) (*DB, error) {
	conn, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	db := &DB{conn: conn}

	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			subdomain_id TEXT NOT NULL,
			client_ip TEXT NOT NULL,
			start_time DATETIME NOT NULL,
			end_time DATETIME,
			duration TEXT,
			message_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS session_metadata (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER,
			subdomain_id TEXT NOT NULL,
			message_count INTEGER DEFAULT 0,
			total_payload INTEGER DEFAULT 0,

			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions (id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_subdomain ON sessions(subdomain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_metadata_subdomain ON session_metadata(subdomain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_metadata_session ON session_metadata(session_id)`,
	}

	for _, query := range queries {
		if _, err := db.conn.Exec(query); err != nil {
			return fmt.Errorf("failed to execute migration: %w", err)
		}
	}

	return nil
}

func (db *DB) CreateSession(subdomainID, clientIP string, startTime time.Time) (int64, error) {
	query := `INSERT INTO sessions (subdomain_id, client_ip, start_time) VALUES (?, ?, ?)`
	result, err := db.conn.Exec(query, subdomainID, clientIP, startTime)
	if err != nil {
		return 0, fmt.Errorf("failed to create session: %w", err)
	}

	return result.LastInsertId()
}

func (db *DB) SaveMetadata(sessionID int64, subdomainID string, messageCount int, totalPayload int64) error {
	query := `INSERT INTO session_metadata (session_id, subdomain_id, message_count, total_payload) VALUES (?, ?, ?, ?)`
	_, err := db.conn.Exec(query, sessionID, subdomainID, messageCount, totalPayload)
	if err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}
	return nil
}

func (db *DB) GetMetadataBySubdomain(subdomainID string, limit int) (models.Metadata, error) {
	query := `SELECT id, session_id, subdomain_id, message_count, total_payload, created_at 
			  FROM session_metadata 
			  WHERE subdomain_id = ? 
			  ORDER BY created_at DESC 
			  LIMIT ?`

	rows, err := db.conn.Query(query, subdomainID, limit)
	if err != nil {
		return models.Metadata{}, fmt.Errorf("failed to get metadata: %w", err)
	}
	defer rows.Close()

	var metadata models.Metadata

	for rows.Next() {
		err := rows.Scan(&metadata.ID, &metadata.SessionID, &metadata.SubdomainID, &metadata.MessageCount, &metadata.TotalPayload, &metadata.CreatedAt)
		if err != nil {
			return models.Metadata{}, fmt.Errorf("failed to scan metadata: %w", err)
		}
	}

	return metadata, nil
}

func (db *DB) EndSession(sessionID int64, endTime time.Time, duration string, messageCount int) error {
	query := `UPDATE sessions SET end_time = ?, duration = ?, message_count = ? WHERE id = ?`
	_, err := db.conn.Exec(query, endTime, duration, messageCount, sessionID)
	if err != nil {
		return fmt.Errorf("failed to end session: %w", err)
	}

	return nil
}
