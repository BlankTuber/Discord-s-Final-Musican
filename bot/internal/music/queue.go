package music

import (
	"database/sql"
	"fmt"
	"musicbot/internal/config"
	"musicbot/internal/logger"
	"musicbot/internal/state"
	"sync"
)

type Queue struct {
	items     []state.QueueItem
	position  int
	dbManager *config.DatabaseManager
	mu        sync.RWMutex
}

func NewQueue(dbManager *config.DatabaseManager) *Queue {
	q := &Queue{
		items:     make([]state.QueueItem, 0),
		position:  0,
		dbManager: dbManager,
	}

	q.loadFromDatabase()
	return q
}

func (q *Queue) loadFromDatabase() {
	items, err := q.dbManager.GetQueue()
	if err != nil {
		logger.Error.Printf("Failed to load queue from database: %v", err)
		return
	}

	position, err := q.dbManager.GetCurrentQueuePosition()
	if err != nil {
		logger.Error.Printf("Failed to load queue position from database: %v", err)
		position = 0
	}

	q.mu.Lock()
	q.items = items
	q.position = position
	q.mu.Unlock()

	logger.Info.Printf("Loaded queue with %d songs, position: %d", len(items), position)
}

func (q *Queue) Add(song *state.Song) error {
	var songID int64

	existing, err := q.dbManager.GetSongByURL(song.URL)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check for existing song: %w", err)
	}

	if existing != nil {
		songID = existing.ID
		song.ID = songID
		logger.Info.Printf("Using existing song from database: %s (ID: %d)", song.Title, songID)
	} else {
		songID, err = q.dbManager.AddSong(song)
		if err != nil {
			return fmt.Errorf("failed to add song to database: %w", err)
		}
		song.ID = songID
		logger.Info.Printf("Added new song to database: %s (ID: %d)", song.Title, songID)
	}

	err = q.dbManager.AddToQueue(songID)
	if err != nil {
		return fmt.Errorf("failed to add song to queue: %w", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	newPosition := len(q.items) + 1
	item := state.QueueItem{
		SongID:   songID,
		Position: newPosition,
		Song:     song,
	}

	q.items = append(q.items, item)

	logger.Info.Printf("Added song to queue: %s by %s", song.Title, song.Artist)
	return nil
}

func (q *Queue) GetCurrent() *state.Song {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.position < 0 || q.position >= len(q.items) {
		return nil
	}

	return q.items[q.position].Song
}

func (q *Queue) GetNext() *state.Song {
	q.mu.RLock()
	defer q.mu.RUnlock()

	nextPos := q.position + 1
	if nextPos >= len(q.items) {
		return nil
	}

	return q.items[nextPos].Song
}

func (q *Queue) Advance() (*state.Song, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.position+1 >= len(q.items) {
		return nil, fmt.Errorf("no more songs in queue")
	}

	q.position++

	err := q.dbManager.SetCurrentQueuePosition(q.position)
	if err != nil {
		logger.Error.Printf("Failed to save queue position: %v", err)
	}

	logger.Info.Printf("Advanced to next song in queue, position: %d", q.position)
	return q.items[q.position].Song, nil
}

func (q *Queue) HasNext() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.position+1 < len(q.items)
}

func (q *Queue) IsEmpty() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.items) == 0
}

func (q *Queue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.items)
}

func (q *Queue) GetPosition() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.position
}

func (q *Queue) GetItems() []state.QueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	items := make([]state.QueueItem, len(q.items))
	copy(items, q.items)
	return items
}

func (q *Queue) GetUpcoming(limit int) []state.Song {
	q.mu.RLock()
	defer q.mu.RUnlock()

	upcoming := make([]state.Song, 0)
	start := q.position + 1
	end := start + limit

	if end > len(q.items) {
		end = len(q.items)
	}

	for i := start; i < end; i++ {
		if q.items[i].Song != nil {
			upcoming = append(upcoming, *q.items[i].Song)
		}
	}

	return upcoming
}

func (q *Queue) Clear() error {
	err := q.dbManager.ClearQueue()
	if err != nil {
		return fmt.Errorf("failed to clear queue in database: %w", err)
	}

	q.mu.Lock()
	q.items = make([]state.QueueItem, 0)
	q.position = 0
	q.mu.Unlock()

	logger.Info.Println("Queue cleared")
	return nil
}

func (q *Queue) Remove(queueID int64) error {
	err := q.dbManager.RemoveFromQueue(queueID)
	if err != nil {
		return fmt.Errorf("failed to remove from queue in database: %w", err)
	}

	q.loadFromDatabase()

	logger.Info.Printf("Removed song from queue: %d", queueID)
	return nil
}
