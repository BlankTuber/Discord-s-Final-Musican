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

	// Test the connection
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