package radio

import (
	"context"
	"musicbot/internal/logger"
	"musicbot/internal/state"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type Manager struct {
	player        *Player
	streamManager *StreamManager
	stateManager  *state.Manager
	starting      bool
	mu            sync.RWMutex
}

func NewManager(stateManager *state.Manager, streams []state.StreamOption) *Manager {
	return &Manager{
		player:        NewPlayer(stateManager),
		streamManager: NewStreamManager(streams),
		stateManager:  stateManager,
	}
}

func (m *Manager) Start(vc *discordgo.VoiceConnection) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.starting {
		logger.Info.Println("Radio is already starting, ignoring duplicate start request")
		return nil
	}

	if m.player.IsPlaying() {
		logger.Info.Println("Radio is already playing, ignoring start request")
		return nil
	}

	m.starting = true
	defer func() {
		m.starting = false
	}()

	logger.Info.Println("Starting radio stream...")
	m.stateManager.SetRadioPlaying(true)

	return m.player.Start(vc)
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.player.IsPlaying() {
		return
	}

	logger.Info.Println("Stopping radio stream...")
	m.player.Stop()
	m.stateManager.SetRadioPlaying(false)
}

func (m *Manager) ChangeStream(streamName string) error {
	stream, err := m.streamManager.GetStreamByName(streamName)
	if err != nil {
		return err
	}

	logger.Info.Printf("Changing radio stream to: %s", streamName)

	wasPlaying := m.player.IsPlaying()
	if wasPlaying {
		m.Stop()
	}

	m.stateManager.SetRadioStream(stream.URL)

	return nil
}

func (m *Manager) GetStreamNames() []string {
	return m.streamManager.GetStreamNames()
}

func (m *Manager) IsValidStream(name string) bool {
	return m.streamManager.IsValidStream(name)
}

func (m *Manager) IsPlaying() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.player.IsPlaying() || m.starting
}

func (m *Manager) Shutdown(ctx context.Context) error {
	logger.Info.Println("Shutting down radio manager...")
	return m.player.Shutdown(ctx)
}

func (m *Manager) Name() string {
	return "RadioManager"
}
