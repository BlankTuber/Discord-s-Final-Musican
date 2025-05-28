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
	player       *Player
	queue        *Queue
	stateManager *state.Manager
	dbManager    *config.DatabaseManager
	socketClient *socket.Client
	radioManager *radio.Manager
	vcGetter     func() *discordgo.VoiceConnection
	mu           sync.RWMutex
}

func NewManager(stateManager *state.Manager, dbManager *config.DatabaseManager, radioManager *radio.Manager, socketClient *socket.Client) *Manager {
	manager := &Manager{
		player:       NewPlayer(stateManager),
		queue:        NewQueue(dbManager),
		stateManager: stateManager,
		dbManager:    dbManager,
		radioManager: radioManager,
		socketClient: socketClient,
	}

	manager.player.SetOnSongEnd(manager.onSongEnd)

	return manager
}

func (m *Manager) RequestSong(url, requestedBy string) error {
	if m.socketClient == nil || !m.socketClient.IsConnected() {
		return fmt.Errorf("downloader not available")
	}

	logger.Info.Printf("Requesting download for: %s", url)

	err := m.socketClient.SendDownloadRequest(url, requestedBy)
	if err != nil {
		return fmt.Errorf("failed to send download request: %w", err)
	}

	return nil
}

func (m *Manager) RequestPlaylist(url, requestedBy string) error {
	if m.socketClient == nil || !m.socketClient.IsConnected() {
		return fmt.Errorf("downloader not available")
	}

	logger.Info.Printf("Requesting playlist download for: %s", url)

	err := m.socketClient.SendPlaylistRequest(url, requestedBy)
	if err != nil {
		return fmt.Errorf("failed to send playlist request: %w", err)
	}

	return nil
}

func (m *Manager) OnDownloadComplete(song *state.Song) error {
	err := m.queue.Add(song)
	if err != nil {
		return fmt.Errorf("failed to add song to queue: %w", err)
	}

	logger.Info.Printf("Song added to queue: %s by %s", song.Title, song.Artist)

	currentState := m.stateManager.GetBotState()

	if currentState == state.StateDJ && !m.player.IsPlaying() {
		go m.startNextSong()
	} else if currentState == state.StateRadio || currentState == state.StateIdle {
		m.radioManager.Stop()
		m.stateManager.SetBotState(state.StateDJ)
		go m.startNextSong()
	}

	return nil
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
}

func (m *Manager) playNext() {
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
}

func (m *Manager) onSongEnd() {
	if m.stateManager.IsShuttingDown() {
		return
	}

	if m.queue.HasNext() {
		go m.playNext()
	} else {
		logger.Info.Println("Queue finished, no more songs")
		m.stateManager.SetBotState(state.StateIdle)
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
