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
	
	CREATE TABLE IF NOT EXISTS songs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		artist TEXT NOT NULL,
		duration INTEGER NOT NULL,
		file_path TEXT NOT NULL,
		url TEXT NOT NULL,
		requested_by TEXT NOT NULL,
		added_at DATETIME NOT NULL
	);
	
	CREATE TABLE IF NOT EXISTS queue (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		song_id INTEGER NOT NULL,
		position INTEGER NOT NULL,
		FOREIGN KEY (song_id) REFERENCES songs (id)
	);
	
	CREATE TABLE IF NOT EXISTS queue_state (
		key TEXT PRIMARY KEY,
		value INTEGER NOT NULL
	);
	
	INSERT OR IGNORE INTO config (key, value) VALUES 
		('volume', '0.05'),
		('stream', 'https://listen.moe/stream');
		
	INSERT OR IGNORE INTO queue_state (key, value) VALUES 
		('current_position', '0');
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

func (dm *DatabaseManager) AddSong(song *state.Song) (int64, error) {
	result, err := dm.db.Exec(`
		INSERT INTO songs (title, artist, duration, file_path, url, requested_by, added_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, song.Title, song.Artist, song.Duration, song.FilePath, song.URL, song.RequestedBy, song.AddedAt)

	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

func (dm *DatabaseManager) AddToQueue(songID int64) error {
	maxPos := 0
	err := dm.db.QueryRow("SELECT COALESCE(MAX(position), 0) FROM queue").Scan(&maxPos)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	_, err = dm.db.Exec("INSERT INTO queue (song_id, position) VALUES (?, ?)", songID, maxPos+1)
	return err
}

func (dm *DatabaseManager) GetQueue() ([]state.QueueItem, error) {
	rows, err := dm.db.Query(`
		SELECT q.id, q.song_id, q.position, s.title, s.artist, s.duration, s.file_path, s.url, s.requested_by, s.added_at
		FROM queue q
		JOIN songs s ON q.song_id = s.id
		ORDER BY q.position
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queue []state.QueueItem
	for rows.Next() {
		var item state.QueueItem
		var song state.Song

		err := rows.Scan(&item.ID, &item.SongID, &item.Position,
			&song.Title, &song.Artist, &song.Duration, &song.FilePath,
			&song.URL, &song.RequestedBy, &song.AddedAt)
		if err != nil {
			continue
		}

		song.ID = item.SongID
		item.Song = &song
		queue = append(queue, item)
	}

	return queue, nil
}

func (dm *DatabaseManager) GetCurrentQueuePosition() (int, error) {
	var position int
	err := dm.db.QueryRow("SELECT value FROM queue_state WHERE key = 'current_position'").Scan(&position)
	return position, err
}

func (dm *DatabaseManager) SetCurrentQueuePosition(position int) error {
	_, err := dm.db.Exec("UPDATE queue_state SET value = ? WHERE key = 'current_position'", position)
	return err
}

func (dm *DatabaseManager) ClearQueue() error {
	_, err := dm.db.Exec("DELETE FROM queue")
	if err != nil {
		return err
	}

	return dm.SetCurrentQueuePosition(0)
}

func (dm *DatabaseManager) RemoveFromQueue(queueID int64) error {
	_, err := dm.db.Exec("DELETE FROM queue WHERE id = ?", queueID)
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
