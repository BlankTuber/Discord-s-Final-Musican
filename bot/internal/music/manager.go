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
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Manager struct {
	player              *Player
	queue               *Queue
	stateManager        *state.Manager
	dbManager           *config.DatabaseManager
	socketClient        *socket.Client
	radioManager        *radio.Manager
	vcGetter            func() *discordgo.VoiceConnection
	activeDownloads     map[string]bool
	activePlaylistUrls  map[string]bool
	pendingDownloads    int32
	clearing            int32
	disableAutoHandlers int32
	mu                  sync.RWMutex
	downloadMu          sync.RWMutex
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

func (m *Manager) EnableAutoHandlers() {
	atomic.StoreInt32(&m.disableAutoHandlers, 0)
	logger.Debug.Println("Auto handlers enabled")
}

func (m *Manager) DisableAutoHandlers() {
	atomic.StoreInt32(&m.disableAutoHandlers, 1)
	logger.Debug.Println("Auto handlers disabled")
}

func (m *Manager) AreAutoHandlersEnabled() bool {
	return atomic.LoadInt32(&m.disableAutoHandlers) == 0
}

func (m *Manager) ExecuteWithDisabledHandlers(fn func()) {
	m.DisableAutoHandlers()
	defer m.EnableAutoHandlers()
	fn()
}

func (m *Manager) Pause() error {
	if !m.player.IsPlaying() {
		return fmt.Errorf("no song is currently playing")
	}

	if m.player.IsPaused() {
		return fmt.Errorf("music is already paused")
	}

	logger.Info.Println("Pausing music...")
	m.player.Pause()

	return nil
}

func (m *Manager) Resume() error {
	if !m.player.IsPaused() {
		return fmt.Errorf("music is not paused")
	}

	vc := m.getVoiceConnection()
	if vc == nil {
		return fmt.Errorf("no voice connection available")
	}

	logger.Info.Println("Resuming music...")
	return m.player.Resume(vc)
}

func (m *Manager) IsPaused() bool {
	return m.player.IsPaused()
}

func (m *Manager) RequestSong(url, requestedBy string) error {
	if atomic.LoadInt32(&m.clearing) == 1 {
		logger.Info.Printf("Ignoring song request while clearing queue: %s", url)
		return nil
	}

	if m.socketClient == nil || !m.socketClient.IsConnected() {
		return fmt.Errorf("downloader not available")
	}

	m.downloadMu.Lock()
	if m.activeDownloads[url] {
		m.downloadMu.Unlock()
		logger.Info.Printf("Song already being downloaded: %s", url)
		return nil
	}
	m.activeDownloads[url] = true
	m.downloadMu.Unlock()

	atomic.AddInt32(&m.pendingDownloads, 1)
	logger.Info.Printf("Requesting download for: %s (pending: %d)", url, atomic.LoadInt32(&m.pendingDownloads))

	go func() {
		defer func() {
			m.downloadMu.Lock()
			delete(m.activeDownloads, url)
			m.downloadMu.Unlock()
		}()

		err := m.socketClient.SendDownloadRequest(url, requestedBy)
		if err != nil {
			atomic.AddInt32(&m.pendingDownloads, -1)
			logger.Error.Printf("Failed to send download request: %v", err)
		}
	}()

	return nil
}

func (m *Manager) RequestPlaylist(url, requestedBy string, limit int) error {
	if atomic.LoadInt32(&m.clearing) == 1 {
		logger.Info.Printf("Ignoring playlist request while clearing queue: %s", url)
		return nil
	}

	if m.socketClient == nil || !m.socketClient.IsConnected() {
		return fmt.Errorf("downloader not available")
	}

	m.downloadMu.Lock()
	if m.activePlaylistUrls[url] {
		m.downloadMu.Unlock()
		logger.Info.Printf("Playlist already being downloaded: %s", url)
		return nil
	}
	m.activePlaylistUrls[url] = true
	m.downloadMu.Unlock()

	logger.Info.Printf("Requesting playlist download for: %s (limit: %d)", url, limit)

	go func() {
		defer func() {
			m.downloadMu.Lock()
			delete(m.activePlaylistUrls, url)
			m.downloadMu.Unlock()
		}()

		err := m.socketClient.SendPlaylistRequest(url, requestedBy, limit)
		if err != nil {
			logger.Error.Printf("Failed to send playlist request: %v", err)
		}
	}()

	return nil
}

func (m *Manager) OnPlaylistStart(totalTracks int) {
	if atomic.LoadInt32(&m.clearing) == 1 {
		logger.Info.Printf("Ignoring playlist start while clearing queue, tracks: %d", totalTracks)
		return
	}

	atomic.AddInt32(&m.pendingDownloads, int32(totalTracks))
	logger.Info.Printf("Playlist started with %d tracks (total pending: %d)", totalTracks, atomic.LoadInt32(&m.pendingDownloads))
}

func (m *Manager) OnDownloadComplete(song *state.Song) error {
	atomic.AddInt32(&m.pendingDownloads, -1)

	if song == nil {
		logger.Info.Printf("Download failed, decremented pending counter (pending: %d)", atomic.LoadInt32(&m.pendingDownloads))
		return nil
	}

	if atomic.LoadInt32(&m.clearing) == 1 {
		logger.Info.Printf("Ignoring download completion while clearing queue: %s (pending: %d)", song.Title, atomic.LoadInt32(&m.pendingDownloads))
		return nil
	}

	go func() {
		err := m.queue.Add(song)
		if err != nil {
			logger.Error.Printf("Failed to add song to queue: %v", err)
			return
		}

		logger.Info.Printf("Song added to queue: %s by %s (pending: %d)", song.Title, song.Artist, atomic.LoadInt32(&m.pendingDownloads))

		if atomic.LoadInt32(&m.clearing) == 0 {
			m.handleQueueAddition()
		}
	}()

	return nil
}

func (m *Manager) OnPlaylistItemComplete(playlistUrl string, song *state.Song) error {
	return m.OnDownloadComplete(song)
}

func (m *Manager) handleQueueAddition() {
	if atomic.LoadInt32(&m.clearing) == 1 {
		return
	}

	currentState := m.stateManager.GetBotState()

	if currentState == state.StateDJ && !m.player.IsPlaying() && !m.player.IsPaused() {
		m.startNextSong()
	} else if currentState == state.StateRadio || currentState == state.StateIdle {
		m.radioManager.Stop()
		time.Sleep(200 * time.Millisecond)
		m.stateManager.SetBotState(state.StateDJ)
		m.startNextSong()
	}
}

func (m *Manager) Start(vc *discordgo.VoiceConnection) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if atomic.LoadInt32(&m.clearing) == 1 {
		return fmt.Errorf("cannot start music while clearing queue")
	}

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

	if !m.player.IsPlaying() && !m.player.IsPaused() {
		return
	}

	logger.Info.Println("Stopping music...")
	m.player.Stop()
}

func (m *Manager) startNextSong() {
	if atomic.LoadInt32(&m.clearing) == 1 {
		return
	}

	go func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		if m.stateManager.IsShuttingDown() || atomic.LoadInt32(&m.clearing) == 1 {
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
	if atomic.LoadInt32(&m.clearing) == 1 {
		return
	}

	go func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		if m.stateManager.IsShuttingDown() || atomic.LoadInt32(&m.clearing) == 1 {
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
	if m.stateManager.IsShuttingDown() || atomic.LoadInt32(&m.clearing) == 1 {
		return
	}

	if !m.AreAutoHandlersEnabled() {
		logger.Debug.Println("Auto handlers disabled, skipping onSongEnd")
		return
	}

	if m.stateManager.IsManualOperationActive() {
		logger.Debug.Println("Manual operation active, skipping onSongEnd")
		return
	}

	if m.queue.HasNext() {
		m.playNext()
	} else {
		logger.Info.Println("Queue finished, no more songs")

		go func() {
			time.Sleep(1 * time.Second)

			if atomic.LoadInt32(&m.clearing) == 1 {
				return
			}

			if !m.AreAutoHandlersEnabled() {
				return
			}

			if m.stateManager.IsManualOperationActive() {
				return
			}

			if m.stateManager.IsInIdleChannel() {
				m.stateManager.SetBotState(state.StateIdle)
			} else {
				m.stateManager.SetBotState(state.StateRadio)
			}

			time.Sleep(500 * time.Millisecond)

			vc := m.getVoiceConnection()
			if vc != nil && !m.radioManager.IsPlaying() {
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

func (m *Manager) HasActiveDownloads() bool {
	m.downloadMu.RLock()
	activeRequests := len(m.activeDownloads) > 0 || len(m.activePlaylistUrls) > 0
	m.downloadMu.RUnlock()

	pendingCount := atomic.LoadInt32(&m.pendingDownloads)

	if activeRequests || pendingCount > 0 {
		logger.Info.Printf("Active downloads check: requests=%v, pending=%d", activeRequests, pendingCount)
	}

	return activeRequests || pendingCount > 0
}

func (m *Manager) ResetPendingDownloads() {
	old := atomic.SwapInt32(&m.pendingDownloads, 0)
	if old > 0 {
		logger.Info.Printf("Reset pending downloads counter from %d to 0", old)
	}
}

func (m *Manager) GetPendingDownloads() int {
	return int(atomic.LoadInt32(&m.pendingDownloads))
}

func (m *Manager) ClearQueue() error {
	if m.HasActiveDownloads() {
		return fmt.Errorf("cannot clear queue while downloads are in progress")
	}

	atomic.StoreInt32(&m.clearing, 1)
	defer atomic.StoreInt32(&m.clearing, 0)

	m.Stop()

	time.Sleep(1 * time.Second)

	err := m.queue.Clear()
	if err != nil {
		return err
	}

	atomic.StoreInt32(&m.pendingDownloads, 0)
	logger.Info.Println("Cleared pending downloads counter")

	time.Sleep(500 * time.Millisecond)

	return nil
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
