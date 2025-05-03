package database

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/logger"
)

type Manager struct {
	dbPath string
	db     *sql.DB
	mu     sync.Mutex
}

func NewManager(dbPath string) (*Manager, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Manager{
		dbPath: dbPath,
		db:     db,
	}, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

func (m *Manager) GetTrackByURL(url string) (*audio.Track, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `SELECT id, title, url, platform, file_path, duration, file_size, 
			thumbnail_url, artist, download_date, play_count, last_played, is_stream 
			FROM songs WHERE url = ?`

	row := m.db.QueryRow(query, url)

	var id int64
	var title, platform, filePath, thumbnailURL, artist string
	var duration, fileSize, downloadDate, playCount, lastPlayed int64
	var isStream bool

	err := row.Scan(&id, &title, &url, &platform, &filePath, &duration, &fileSize,
		&thumbnailURL, &artist, &downloadDate, &playCount, &lastPlayed, &isStream)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("error querying track: %w", err)
	}

	track := &audio.Track{
		Title:        title,
		URL:          url,
		Duration:     int(duration),
		FilePath:     filePath,
		ArtistName:   artist,
		ThumbnailURL: thumbnailURL,
		IsStream:     isStream,
	}

	return track, nil
}

func (m *Manager) GetTrackByFilePath(filePath string) (*audio.Track, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `SELECT id, title, url, platform, duration, file_size, 
			thumbnail_url, artist, download_date, play_count, last_played, is_stream 
			FROM songs WHERE file_path = ?`

	row := m.db.QueryRow(query, filePath)

	var id int64
	var title, url, platform, thumbnailURL, artist string
	var duration, fileSize, downloadDate, playCount, lastPlayed int64
	var isStream bool

	err := row.Scan(&id, &title, &url, &platform, &duration, &fileSize,
		&thumbnailURL, &artist, &downloadDate, &playCount, &lastPlayed, &isStream)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("error querying track: %w", err)
	}

	track := &audio.Track{
		Title:        title,
		URL:          url,
		Duration:     int(duration),
		FilePath:     filePath,
		ArtistName:   artist,
		ThumbnailURL: thumbnailURL,
		IsStream:     isStream,
	}

	return track, nil
}

func (m *Manager) IncrementPlayCount(url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentTime := time.Now().Unix()

	query := `UPDATE songs SET play_count = play_count + 1, last_played = ? WHERE url = ?`

	_, err := m.db.Exec(query, currentTime, url)
	if err != nil {
		return fmt.Errorf("error updating play count: %w", err)
	}

	return nil
}

func (m *Manager) GetPopularTracks(limit int) ([]*audio.Track, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `SELECT id, title, url, platform, file_path, duration, file_size, 
			thumbnail_url, artist, download_date, play_count, last_played, is_stream 
			FROM songs ORDER BY play_count DESC LIMIT ?`

	rows, err := m.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("error querying popular tracks: %w", err)
	}
	defer rows.Close()

	tracks := make([]*audio.Track, 0, limit)

	for rows.Next() {
		var id int64
		var title, url, platform, filePath, thumbnailURL, artist string
		var duration, fileSize, downloadDate, playCount, lastPlayed int64
		var isStream bool

		err := rows.Scan(&id, &title, &url, &platform, &filePath, &duration, &fileSize,
			&thumbnailURL, &artist, &downloadDate, &playCount, &lastPlayed, &isStream)

		if err != nil {
			logger.ErrorLogger.Printf("Error scanning track row: %v", err)
			continue
		}

		track := &audio.Track{
			Title:        title,
			URL:          url,
			Duration:     int(duration),
			FilePath:     filePath,
			ArtistName:   artist,
			ThumbnailURL: thumbnailURL,
			IsStream:     isStream,
		}

		tracks = append(tracks, track)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tracks: %w", err)
	}

	return tracks, nil
}

func (m *Manager) GetRecentTracks(limit int) ([]*audio.Track, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `SELECT id, title, url, platform, file_path, duration, file_size, 
			thumbnail_url, artist, download_date, play_count, last_played, is_stream 
			FROM songs WHERE last_played IS NOT NULL ORDER BY last_played DESC LIMIT ?`

	rows, err := m.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("error querying recent tracks: %w", err)
	}
	defer rows.Close()

	tracks := make([]*audio.Track, 0, limit)

	for rows.Next() {
		var id int64
		var title, url, platform, filePath, thumbnailURL, artist string
		var duration, fileSize, downloadDate, playCount, lastPlayed int64
		var isStream bool

		err := rows.Scan(&id, &title, &url, &platform, &filePath, &duration, &fileSize,
			&thumbnailURL, &artist, &downloadDate, &playCount, &lastPlayed, &isStream)

		if err != nil {
			logger.ErrorLogger.Printf("Error scanning track row: %v", err)
			continue
		}

		track := &audio.Track{
			Title:        title,
			URL:          url,
			Duration:     int(duration),
			FilePath:     filePath,
			ArtistName:   artist,
			ThumbnailURL: thumbnailURL,
			IsStream:     isStream,
		}

		tracks = append(tracks, track)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tracks: %w", err)
	}

	return tracks, nil
}

func (m *Manager) GetQueue(guildID string) ([]*audio.Track, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var queueID int64
	queryQueue := `SELECT id FROM queues WHERE guild_id = ?`
	err := m.db.QueryRow(queryQueue, guildID).Scan(&queueID)
	if err == sql.ErrNoRows {
		return []*audio.Track{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error querying queue: %w", err)
	}

	queryItems := `
		SELECT qi.title, qi.url, qi.duration, qi.requester, qi.requested_at, s.file_path, s.thumbnail_url, s.artist, s.is_stream
		FROM queue_items qi
		LEFT JOIN songs s ON qi.song_id = s.id
		WHERE qi.queue_id = ? AND qi.played = 0
		ORDER BY qi.position ASC`

	rows, err := m.db.Query(queryItems, queueID)
	if err != nil {
		return nil, fmt.Errorf("error querying queue items: %w", err)
	}
	defer rows.Close()

	tracks := []*audio.Track{}
	for rows.Next() {
		var title, url, requester, filePath, thumbnailURL, artist sql.NullString
		var duration, requestedAt sql.NullInt64
		var isStream sql.NullBool

		err := rows.Scan(&title, &url, &duration, &requester, &requestedAt, &filePath, &thumbnailURL, &artist, &isStream)
		if err != nil {
			logger.ErrorLogger.Printf("Error scanning queue item: %v", err)
			continue
		}

		track := &audio.Track{
			Title:        title.String,
			URL:          url.String,
			Duration:     int(duration.Int64),
			Requester:    requester.String,
			RequestedAt:  requestedAt.Int64,
			FilePath:     filePath.String,
			ThumbnailURL: thumbnailURL.String,
			ArtistName:   artist.String,
			IsStream:     isStream.Bool,
		}
		tracks = append(tracks, track)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating queue items: %w", err)
	}

	return tracks, nil
}

func (m *Manager) SaveQueue(guildID string, tracks []*audio.Track) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("error beginning transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	currentTime := time.Now().Unix()

	var queueID int64
	queryQueue := `SELECT id FROM queues WHERE guild_id = ?`
	err = tx.QueryRow(queryQueue, guildID).Scan(&queueID)
	if err == sql.ErrNoRows {
		result, err := tx.Exec(`
			INSERT INTO queues (guild_id, created_at, updated_at)
			VALUES (?, ?, ?)`, guildID, currentTime, currentTime)
		if err != nil {
			return fmt.Errorf("error creating queue: %w", err)
		}

		queueID, err = result.LastInsertId()
		if err != nil {
			return fmt.Errorf("error getting queue id: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("error checking existing queue: %w", err)
	} else {
		_, err = tx.Exec(`
			UPDATE queues SET updated_at = ? WHERE id = ?`, currentTime, queueID)
		if err != nil {
			return fmt.Errorf("error updating queue: %w", err)
		}

		_, err = tx.Exec(`
			DELETE FROM queue_items WHERE queue_id = ? AND played = 0`, queueID)
		if err != nil {
			return fmt.Errorf("error clearing queue items: %w", err)
		}
	}

	for i, track := range tracks {
		var songID *int64
		if track.URL != "" {
			var id int64
			err = tx.QueryRow(`SELECT id FROM songs WHERE url = ?`, track.URL).Scan(&id)
			if err == nil {
				songID = &id
			} else if err != sql.ErrNoRows {
				return fmt.Errorf("error querying song: %w", err)
			}
		}

		var songIDValue interface{}
		if songID != nil {
			songIDValue = *songID
		} else {
			songIDValue = nil
		}

		_, err = tx.Exec(`
			INSERT INTO queue_items (queue_id, song_id, title, url, duration, requester, requested_at, position, played)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			queueID, songIDValue, track.Title, track.URL, track.Duration, track.Requester, track.RequestedAt, i)
		if err != nil {
			return fmt.Errorf("error inserting queue item: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

func (m *Manager) MarkQueueItemPlayed(guildID string, position int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var queueID int64
	queryQueue := `SELECT id FROM queues WHERE guild_id = ?`
	err := m.db.QueryRow(queryQueue, guildID).Scan(&queueID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error querying queue: %w", err)
	}

	_, err = m.db.Exec(`
		UPDATE queue_items 
		SET played = 1 
		WHERE queue_id = ? AND position = ?`,
		queueID, position)
	if err != nil {
		return fmt.Errorf("error marking queue item as played: %w", err)
	}

	return nil
}

func (m *Manager) RemoveQueueItem(guildID string, position int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var queueID int64
	queryQueue := `SELECT id FROM queues WHERE guild_id = ?`
	err := m.db.QueryRow(queryQueue, guildID).Scan(&queueID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error querying queue: %w", err)
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("error beginning transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	_, err = tx.Exec(`
		DELETE FROM queue_items 
		WHERE queue_id = ? AND position = ? AND played = 0`,
		queueID, position)
	if err != nil {
		return fmt.Errorf("error removing queue item: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE queue_items 
		SET position = position - 1 
		WHERE queue_id = ? AND position > ? AND played = 0`,
		queueID, position)
	if err != nil {
		return fmt.Errorf("error reordering queue items: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

func (m *Manager) ClearQueue(guildID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var queueID int64
	queryQueue := `SELECT id FROM queues WHERE guild_id = ?`
	err := m.db.QueryRow(queryQueue, guildID).Scan(&queueID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error querying queue: %w", err)
	}

	_, err = m.db.Exec(`
		DELETE FROM queue_items 
		WHERE queue_id = ? AND played = 0`,
		queueID)
	if err != nil {
		return fmt.Errorf("error clearing queue: %w", err)
	}

	return nil
}