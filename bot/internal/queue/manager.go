package queue

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/database"
	"quidque.com/discord-musican/internal/logger"
)

type QueueEventType string

const (
	EventTrackAdded      QueueEventType = "track_added"
	EventTrackRemoved    QueueEventType = "track_removed"
	EventQueueCleared    QueueEventType = "queue_cleared"
	EventTracksAdded     QueueEventType = "tracks_added"
	EventPlaybackStarted QueueEventType = "playback_started"
	EventPlaybackStopped QueueEventType = "playback_stopped"
	EventPlaybackPaused  QueueEventType = "playback_paused"
	EventPlaybackResumed QueueEventType = "playback_resumed"
)

type QueueEvent struct {
	Type     QueueEventType
	GuildID  string
	Track    *audio.Track
	Tracks   []*audio.Track
	Position int
	Count    int
}

type QueueEventCallback func(event QueueEvent)

type Manager struct {
	queues        map[string][]*audio.Track
	currentTracks map[string]*audio.Track
	dbManager     *database.Manager
	eventCallback QueueEventCallback
	mu            sync.RWMutex
}

func NewManager(dbManager *database.Manager) *Manager {
	return &Manager{
		queues:        make(map[string][]*audio.Track),
		currentTracks: make(map[string]*audio.Track),
		dbManager:     dbManager,
	}
}

func (m *Manager) SetEventCallback(callback QueueEventCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventCallback = callback
}

func (m *Manager) fireEvent(event QueueEvent) {
	m.mu.RLock()
	callback := m.eventCallback
	m.mu.RUnlock()

	if callback != nil {
		go callback(event)
	}
}

func (m *Manager) AddTrack(guildID string, track *audio.Track) {
	if track == nil {
		return
	}

	// Verify file exists before adding to queue
	if track.FilePath != "" {
		if _, err := os.Stat(track.FilePath); os.IsNotExist(err) {
			logger.WarnLogger.Printf("Skipping track with missing file: %s", track.FilePath)
			return
		}
	}

	m.mu.Lock()
	if _, ok := m.queues[guildID]; !ok {
		m.queues[guildID] = make([]*audio.Track, 0)
	}

	m.queues[guildID] = append(m.queues[guildID], track)
	m.mu.Unlock()

	// Save the queue to the database
	if m.dbManager != nil {
		go func() {
			err := m.dbManager.SaveQueue(guildID, m.GetQueue(guildID))
			if err != nil {
				logger.ErrorLogger.Printf("Failed to save queue to database: %v", err)
			}
		}()
	}

	m.fireEvent(QueueEvent{
		Type:    EventTrackAdded,
		GuildID: guildID,
		Track:   track,
	})
}

func (m *Manager) AddTracks(guildID string, tracks []*audio.Track) int {
	if len(tracks) == 0 {
		return 0
	}

	validTracks := make([]*audio.Track, 0, len(tracks))

	// Verify files exist before adding to queue
	for _, track := range tracks {
		if track == nil || track.FilePath == "" {
			continue
		}

		if _, err := os.Stat(track.FilePath); os.IsNotExist(err) {
			logger.WarnLogger.Printf("Skipping track with missing file: %s", track.FilePath)
			continue
		}

		validTracks = append(validTracks, track)
	}

	if len(validTracks) == 0 {
		return 0
	}

	m.mu.Lock()
	if _, ok := m.queues[guildID]; !ok {
		m.queues[guildID] = make([]*audio.Track, 0)
	}

	m.queues[guildID] = append(m.queues[guildID], validTracks...)
	m.mu.Unlock()

	// Save the queue to the database
	if m.dbManager != nil {
		go func() {
			err := m.dbManager.SaveQueue(guildID, m.GetQueue(guildID))
			if err != nil {
				logger.ErrorLogger.Printf("Failed to save queue to database: %v", err)
			}
		}()
	}

	m.fireEvent(QueueEvent{
		Type:    EventTracksAdded,
		GuildID: guildID,
		Tracks:  validTracks,
		Count:   len(validTracks),
	})

	return len(validTracks)
}

func (m *Manager) GetQueue(guildID string) []*audio.Track {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if queue, ok := m.queues[guildID]; ok {
		result := make([]*audio.Track, len(queue))
		copy(result, queue)
		return result
	}

	return make([]*audio.Track, 0)
}

func (m *Manager) GetQueueLength(guildID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if queue, ok := m.queues[guildID]; ok {
		return len(queue)
	}

	return 0
}

func (m *Manager) GetNextTrack(guildID string) *audio.Track {
	m.mu.Lock()
	defer m.mu.Unlock()

	if queue, ok := m.queues[guildID]; ok && len(queue) > 0 {
		track := queue[0]
		m.queues[guildID] = queue[1:]

		// Save the updated queue to the database
		if m.dbManager != nil {
			go func() {
				err := m.dbManager.SaveQueue(guildID, m.queues[guildID])
				if err != nil {
					logger.ErrorLogger.Printf("Failed to save queue to database: %v", err)
				}
			}()
		}

		m.currentTracks[guildID] = track
		return track
	}

	return nil
}

func (m *Manager) PeekNextTrack(guildID string) *audio.Track {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if queue, ok := m.queues[guildID]; ok && len(queue) > 0 {
		return queue[0]
	}

	return nil
}

func (m *Manager) RemoveTrack(guildID string, position int) (*audio.Track, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if queue, ok := m.queues[guildID]; ok {
		if position < 0 || position >= len(queue) {
			return nil, fmt.Errorf("invalid position: %d", position)
		}

		track := queue[position]
		m.queues[guildID] = append(queue[:position], queue[position+1:]...)

		// Save the updated queue to the database
		if m.dbManager != nil {
			go func() {
				err := m.dbManager.SaveQueue(guildID, m.queues[guildID])
				if err != nil {
					logger.ErrorLogger.Printf("Failed to save queue to database: %v", err)
				}
			}()
		}

		m.fireEvent(QueueEvent{
			Type:     EventTrackRemoved,
			GuildID:  guildID,
			Track:    track,
			Position: position,
		})

		return track, nil
	}

	return nil, errors.New("queue not found")
}

func (m *Manager) ClearQueue(guildID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queues[guildID] = make([]*audio.Track, 0)

	// Save the empty queue to the database
	if m.dbManager != nil {
		go func() {
			err := m.dbManager.ClearQueue(guildID)
			if err != nil {
				logger.ErrorLogger.Printf("Failed to clear queue in database: %v", err)
			}
		}()
	}

	m.fireEvent(QueueEvent{
		Type:    EventQueueCleared,
		GuildID: guildID,
	})
}

func (m *Manager) GetCurrentTrack(guildID string) *audio.Track {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if track, ok := m.currentTracks[guildID]; ok {
		return track
	}

	// If no current track in memory, try to get from database
	if m.dbManager != nil {
		track, err := m.dbManager.GetCurrentPlayingTrack(guildID)
		if err == nil && track != nil {
			return track
		}
	}

	return nil
}

func (m *Manager) SetCurrentTrack(guildID string, track *audio.Track) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentTracks[guildID] = track
}

func (m *Manager) ClearCurrentTrack(guildID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.currentTracks, guildID)
}

func (m *Manager) IncrementPlayCount(track *audio.Track) {
	if track == nil || track.URL == "" || m.dbManager == nil {
		return
	}

	go func() {
		err := m.dbManager.IncrementPlayCount(track.URL)
		if err != nil {
			logger.ErrorLogger.Printf("Failed to increment play count: %v", err)
		}
	}()
}

func (m *Manager) LoadQueueFromDatabase(guildID string) error {
	if m.dbManager == nil {
		return errors.New("database manager not available")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Only load if queue is empty
	if queue, ok := m.queues[guildID]; ok && len(queue) > 0 {
		return nil
	}

	// Load queue from database - only unplayed items
	queue, err := m.dbManager.GetQueue(guildID, false)
	if err != nil {
		return fmt.Errorf("error loading queue from database: %w", err)
	}

	// Filter out tracks with missing files
	validTracks := make([]*audio.Track, 0, len(queue))
	for _, track := range queue {
		if track.FilePath == "" {
			continue
		}

		if _, err := os.Stat(track.FilePath); os.IsNotExist(err) {
			logger.WarnLogger.Printf("Skipping track with missing file: %s", track.FilePath)
			continue
		}

		validTracks = append(validTracks, track)
	}

	m.queues[guildID] = validTracks
	return nil
}

func (m *Manager) GetHistory(guildID string, limit int) ([]*audio.Track, error) {
	if m.dbManager == nil {
		return nil, errors.New("database manager not available")
	}

	// For now, reuse the RecentTracks method
	return m.dbManager.GetRecentTracks(limit)
}

func (m *Manager) GetPopularTracks(limit int) ([]*audio.Track, error) {
	if m.dbManager == nil {
		return nil, errors.New("database manager not available")
	}

	return m.dbManager.GetPopularTracks(limit)
}

func (m *Manager) MarkTrackAsPlayed(guildID string, track *audio.Track) {
	if track == nil || m.dbManager == nil {
		return
	}

	// Find the track position
	var position int = -1
	m.mu.RLock()
	for i, t := range m.queues[guildID] {
		if t == track {
			position = i
			break
		}
	}
	m.mu.RUnlock()

	if position >= 0 {
		go func() {
			err := m.dbManager.MarkQueueItemPlayed(guildID, position)
			if err != nil {
				logger.ErrorLogger.Printf("Failed to mark queue item as played: %v", err)
			}
		}()
	}
}
