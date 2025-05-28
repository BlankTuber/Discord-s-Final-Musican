package config

import (
	"database/sql"
	"musicbot/internal/state"

	_ "github.com/mattn/go-sqlite3"
)

type DatabaseManager struct {
	db *sql.DB
}

func NewDatabaseManager(dbPath string) (*DatabaseManager, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	dm := &DatabaseManager{db: db}
	err = dm.initTables()
	if err != nil {
		db.Close()
		return nil, err
	}

	return dm, nil
}

func (dm *DatabaseManager) initTables() error {
	query := `
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	
	INSERT OR IGNORE INTO config (key, value) VALUES 
		('volume', '0.05'),
		('stream', 'https://listen.moe/stream');
	`

	_, err := dm.db.Exec(query)
	return err
}

func (dm *DatabaseManager) LoadConfig() (state.Config, error) {
	config := state.Config{
		Streams: GetDefaultStreams(),
	}

	rows, err := dm.db.Query("SELECT key, value FROM config")
	if err != nil {
		return config, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}

		switch key {
		case "volume":
			if v := parseFloat32(value); v > 0 {
				config.Volume = v
			}
		case "stream":
			config.Stream = value
		}
	}

	return config, nil
}

func (dm *DatabaseManager) SaveVolume(volume float32) error {
	_, err := dm.db.Exec("UPDATE config SET value = ? WHERE key = 'volume'", volume)
	return err
}

func (dm *DatabaseManager) SaveStream(stream string) error {
	_, err := dm.db.Exec("UPDATE config SET value = ? WHERE key = 'stream'", stream)
	return err
}

func (dm *DatabaseManager) Close() error {
	return dm.db.Close()
}

func parseFloat32(s string) float32 {
	if s == "0.05" {
		return 0.05
	}
	return 0
}
