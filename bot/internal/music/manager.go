package music

import (
	"context"
	"fmt"
	"musicbot/internal/config"
	"musicbot/internal/logger"
	"musicbot/internal/radio"
	"musicbot/internal/socket"
	"musicbot/internal/state"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Manager struct {
	player             *Player
	queue              *Queue
	stateManager       *state.Manager
	dbManager          *config.DatabaseManager
	socketClient       *socket.Client
	radioManager       *radio.Manager
	vcGetter           func() *discordgo.VoiceConnection
	activeDownloads    map[string]bool // Track active downloads to prevent duplicates
	activePlaylistUrls map[string]bool // Track active playlist downloads
	mu                 sync.RWMutex
	downloadMu         sync.RWMutex
}

func NewManager(stateManager *state.Manager, dbManager *config.DatabaseManager, radioManager *radio.Manager, socketClient *socket.Client) *Manager {
	manager := &Manager{
		player:             NewPlayer(stateManager),
		queue:              NewQueue(dbManager),
		stateManager:       stateManager,
		dbManager:          dbManager,
		radioManager:       radioManager,
		socketClient:       socketClient,
		activeDownloads:    make(map[string]bool),
		activePlaylistUrls: make(map[string]bool),
	}

	manager.player.SetOnSongEnd(manager.onSongEnd)

	return manager
}

func (m *Manager) RequestSong(url, requestedBy string) error {
	if m.socketClient == nil || !m.socketClient.IsConnected() {
		return fmt.Errorf("downloader not available")
	}

	// Check if this URL is already being downloaded
	m.downloadMu.Lock()
	if m.activeDownloads[url] {
		m.downloadMu.Unlock()
		logger.Info.Printf("Song already being downloaded: %s", url)
		return nil
	}
	m.activeDownloads[url] = true
	m.downloadMu.Unlock()

	logger.Info.Printf("Requesting download for: %s", url)

	go func() {
		defer func() {
			m.downloadMu.Lock()
			delete(m.activeDownloads, url)
			m.downloadMu.Unlock()
		}()

		err := m.socketClient.SendDownloadRequest(url, requestedBy)
		if err != nil {
			logger.Error.Printf("Failed to send download request: %v", err)
		}
	}()

	return nil
}

func (m *Manager) RequestPlaylist(url, requestedBy string) error {
	if m.socketClient == nil || !m.socketClient.IsConnected() {
		return fmt.Errorf("downloader not available")
	}

	// Check if this playlist is already being downloaded
	m.downloadMu.Lock()
	if m.activePlaylistUrls[url] {
		m.downloadMu.Unlock()
		logger.Info.Printf("Playlist already being downloaded: %s", url)
		return nil
	}
	m.activePlaylistUrls[url] = true
	m.downloadMu.Unlock()

	logger.Info.Printf("Requesting playlist download for: %s", url)

	go func() {
		defer func() {
			m.downloadMu.Lock()
			delete(m.activePlaylistUrls, url)
			m.downloadMu.Unlock()
		}()

		err := m.socketClient.SendPlaylistRequest(url, requestedBy)
		if err != nil {
			logger.Error.Printf("Failed to send playlist request: %v", err)
		}
	}()

	return nil
}

func (m *Manager) OnDownloadComplete(song *state.Song) error {
	// Run queue operations in a goroutine to avoid blocking
	go func() {
		err := m.queue.Add(song)
		if err != nil {
			logger.Error.Printf("Failed to add song to queue: %v", err)
			return
		}

		logger.Info.Printf("Song added to queue: %s by %s", song.Title, song.Artist)

		// Handle state transitions
		m.handleQueueAddition()
	}()

	return nil
}

func (m *Manager) OnPlaylistItemComplete(playlistUrl string, song *state.Song) error {
	// Handle individual playlist items as they download
	return m.OnDownloadComplete(song)
}

func (m *Manager) handleQueueAddition() {
	currentState := m.stateManager.GetBotState()

	if currentState == state.StateDJ && !m.player.IsPlaying() {
		// Already in DJ mode but not playing, start the next song
		m.startNextSong()
	} else if currentState == state.StateRadio || currentState == state.StateIdle {
		// Switch from radio/idle to DJ mode
		m.radioManager.Stop()
		m.stateManager.SetBotState(state.StateDJ)
		m.startNextSong()
	}
}

func (m *Manager) Start(vc *discordgo.VoiceConnection) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.player.IsPlaying() {
		return nil
	}

	currentSong := m.queue.GetCurrent()
	if currentSong == nil {
		return fmt.Errorf("no songs in queue")
	}

	m.stateManager.SetBotState(state.StateDJ)

	return m.player.Play(vc, currentSong)
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.player.IsPlaying() {
		return
	}

	logger.Info.Println("Stopping music...")
	m.player.Stop()
}

func (m *Manager) startNextSong() {
	// Use a goroutine to avoid blocking
	go func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		if m.stateManager.IsShuttingDown() {
			return
		}

		currentSong := m.queue.GetCurrent()
		if currentSong == nil {
			logger.Info.Println("No songs available to play")
			return
		}

		vc := m.getVoiceConnection()
		if vc == nil {
			logger.Error.Println("No voice connection available for playback")
			return
		}

		err := m.player.Play(vc, currentSong)
		if err != nil {
			logger.Error.Printf("Failed to start playing song: %v", err)
		}
	}()
}

func (m *Manager) playNext() {
	// Use a goroutine to avoid blocking
	go func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		if m.stateManager.IsShuttingDown() {
			return
		}

		nextSong, err := m.queue.Advance()
		if err != nil {
			logger.Info.Println("No more songs in queue")
			return
		}

		vc := m.getVoiceConnection()
		if vc == nil {
			logger.Error.Println("No voice connection available for next song")
			return
		}

		time.Sleep(500 * time.Millisecond)

		err = m.player.Play(vc, nextSong)
		if err != nil {
			logger.Error.Printf("Failed to play next song: %v", err)
		}
	}()
}

func (m *Manager) onSongEnd() {
	if m.stateManager.IsShuttingDown() {
		return
	}

	if m.queue.HasNext() {
		m.playNext()
	} else {
		logger.Info.Println("Queue finished, no more songs")

		// Handle returning to radio state when queue is empty
		go func() {
			time.Sleep(1 * time.Second) // Brief pause before switching modes

			if m.stateManager.IsInIdleChannel() {
				m.stateManager.SetBotState(state.StateIdle)
			} else {
				m.stateManager.SetBotState(state.StateRadio)
			}

			vc := m.getVoiceConnection()
			if vc != nil {
				m.radioManager.Start(vc)
			}
		}()
	}
}

func (m *Manager) GetQueue() []state.QueueItem {
	return m.queue.GetItems()
}

func (m *Manager) GetUpcoming(limit int) []state.Song {
	return m.queue.GetUpcoming(limit)
}

func (m *Manager) GetCurrentSong() *state.Song {
	return m.player.GetCurrentSong()
}

func (m *Manager) IsPlaying() bool {
	return m.player.IsPlaying()
}

func (m *Manager) ClearQueue() error {
	m.Stop()
	return m.queue.Clear()
}

func (m *Manager) RemoveFromQueue(queueID int64) error {
	return m.queue.Remove(queueID)
}

func (m *Manager) getVoiceConnection() *discordgo.VoiceConnection {
	if m.vcGetter != nil {
		return m.vcGetter()
	}
	return nil
}

func (m *Manager) SetVoiceConnectionGetter(getter func() *discordgo.VoiceConnection) {
	m.vcGetter = getter
}

func (m *Manager) Shutdown(ctx context.Context) error {
	logger.Info.Println("Shutting down music manager...")
	return m.player.Shutdown(ctx)
}

func (m *Manager) Name() string {
	return "MusicManager"
}
