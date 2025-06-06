package state

import (
	"sync"
	"time"
)

type Manager struct {
	botState       BotState
	opState        OperationState
	voiceState     VoiceState
	radioState     RadioState
	musicState     MusicState
	config         Config
	lastActivity   time.Time
	shuttingDown   bool
	manualOpActive bool
	mu             sync.RWMutex
}

func NewManager(config Config) *Manager {
	return &Manager{
		botState: StateIdle,
		voiceState: VoiceState{
			IdleChannel: config.IdleChannel,
		},
		radioState: RadioState{
			CurrentStream: config.Stream,
			Volume:        config.Volume,
		},
		musicState: MusicState{
			QueuePosition: 0,
		},
		config:       config,
		lastActivity: time.Now(),
		shuttingDown: false,
	}
}

func (m *Manager) GetBotState() BotState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.botState
}

func (m *Manager) SetBotState(state BotState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.botState = state
	m.lastActivity = time.Now()
}

func (m *Manager) IsShuttingDown() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.shuttingDown
}

func (m *Manager) SetShuttingDown(shutting bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shuttingDown = shutting
}

func (m *Manager) IsManualOperationActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.manualOpActive
}

func (m *Manager) SetManualOperationActive(active bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manualOpActive = active
}

func (m *Manager) IsOperationInProgress() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return !m.shuttingDown && (m.opState.IsJoining || m.opState.IsLeaving || m.opState.IsStreaming || m.opState.IsPlaying)
}

func (m *Manager) SetJoining(joining bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.shuttingDown {
		m.opState.IsJoining = joining
	}
}

func (m *Manager) SetLeaving(leaving bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.shuttingDown {
		m.opState.IsLeaving = leaving
	}
}

func (m *Manager) SetStreaming(streaming bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.shuttingDown {
		m.opState.IsStreaming = streaming
	}
}

func (m *Manager) SetPlaying(playing bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.shuttingDown {
		m.opState.IsPlaying = playing
		m.musicState.IsPlaying = playing
	}
}

func (m *Manager) GetCurrentChannel() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.voiceState.CurrentChannel
}

func (m *Manager) SetCurrentChannel(channel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.voiceState.CurrentChannel = channel
	if !m.shuttingDown {
		m.lastActivity = time.Now()
	}
}

func (m *Manager) GetIdleChannel() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.voiceState.IdleChannel
}

func (m *Manager) IsInIdleChannel() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.voiceState.CurrentChannel == m.voiceState.IdleChannel
}

func (m *Manager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.voiceState.IsConnected
}

func (m *Manager) SetConnected(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.voiceState.IsConnected = connected
}

func (m *Manager) GetRadioStream() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.radioState.CurrentStream
}

func (m *Manager) SetRadioStream(stream string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.radioState.CurrentStream = stream
	if !m.shuttingDown {
		m.lastActivity = time.Now()
	}
}

func (m *Manager) GetVolume() float32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.radioState.Volume
}

func (m *Manager) SetVolume(volume float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if volume >= 0.01 && volume <= 0.1 {
		m.radioState.Volume = volume
		if !m.shuttingDown {
			m.lastActivity = time.Now()
		}
	}
}

func (m *Manager) IsRadioPlaying() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.radioState.IsPlaying
}

func (m *Manager) SetRadioPlaying(playing bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.shuttingDown {
		m.radioState.IsPlaying = playing
	}
}

func (m *Manager) GetCurrentSong() *Song {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.musicState.CurrentSong
}

func (m *Manager) SetCurrentSong(song *Song) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.musicState.CurrentSong = song
	if !m.shuttingDown {
		m.lastActivity = time.Now()
	}
}

func (m *Manager) IsMusicPlaying() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.musicState.IsPlaying
}

func (m *Manager) IsMusicPaused() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.musicState.IsPaused
}

func (m *Manager) SetMusicPaused(paused bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.shuttingDown {
		m.musicState.IsPaused = paused
		if !paused {
			m.lastActivity = time.Now()
		}
	}
}

func (m *Manager) GetQueuePosition() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.musicState.QueuePosition
}

func (m *Manager) SetQueuePosition(position int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.musicState.QueuePosition = position
	if !m.shuttingDown {
		m.lastActivity = time.Now()
	}
}

func (m *Manager) GetConfig() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

func (m *Manager) UpdateConfig(config Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}
