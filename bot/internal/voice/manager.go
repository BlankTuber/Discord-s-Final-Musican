package voice

import (
	"context"
	"musicbot/internal/logger"
	"musicbot/internal/state"

	"github.com/bwmarrin/discordgo"
)

type Manager struct {
	operations   *Operations
	stateManager *state.Manager
}

func NewManager(session *discordgo.Session, stateManager *state.Manager) *Manager {
	return &Manager{
		operations:   NewOperations(session, stateManager),
		stateManager: stateManager,
	}
}

func (m *Manager) JoinUser(guildID, userID string) error {
	if m.stateManager.IsShuttingDown() {
		logger.Debug.Println("Ignoring join user request during shutdown")
		return nil
	}

	logger.Info.Printf("Attempting to join user %s in guild %s", userID, guildID)
	return m.operations.JoinUserChannel(guildID, userID)
}

func (m *Manager) LeaveToIdle(guildID string) error {
	if m.stateManager.IsShuttingDown() {
		logger.Debug.Println("Ignoring leave to idle request during shutdown")
		return nil
	}

	logger.Info.Printf("Leaving to idle channel in guild %s", guildID)
	return m.operations.LeaveToIdle(guildID)
}

func (m *Manager) ReturnToIdle(guildID string) error {
	if m.stateManager.IsShuttingDown() {
		logger.Debug.Println("Ignoring return to idle request during shutdown")
		return nil
	}

	logger.Info.Printf("Returning to idle channel in guild %s", guildID)
	return m.operations.ReturnToIdle(guildID)
}

func (m *Manager) HandleUserLeft(guildID, channelID string) error {
	if m.stateManager.IsShuttingDown() {
		logger.Debug.Println("Ignoring user left event during shutdown")
		return nil
	}

	if m.stateManager.IsInIdleChannel() {
		logger.Info.Println("Already in idle channel, no action needed")
		return nil
	}

	userCount, err := m.operations.CheckChannelUsers(guildID, channelID)
	if err != nil {
		logger.Error.Printf("Error checking channel users: %v", err)
		return err
	}

	logger.Info.Printf("Channel %s has %d users remaining", channelID, userCount)

	if userCount == 0 {
		logger.Info.Println("Channel is empty, returning to idle")
		return m.operations.ReturnToIdle(guildID)
	}

	return nil
}

func (m *Manager) HandleDisconnect(guildID string) error {
	if m.stateManager.IsShuttingDown() {
		logger.Info.Println("Expected disconnect during shutdown, not reconnecting")
		m.operations.GetConnection().HandleDisconnect()
		return nil
	}

	logger.Info.Printf("Handling unexpected disconnect in guild %s", guildID)

	m.operations.GetConnection().HandleDisconnect()

	idleChannel := m.stateManager.GetIdleChannel()
	if idleChannel == "" {
		logger.Error.Println("No idle channel configured")
		return nil
	}

	return m.operations.GetConnection().Join(guildID, idleChannel)
}

func (m *Manager) GetVoiceConnection() *discordgo.VoiceConnection {
	return m.operations.GetConnection().GetConnection()
}

func (m *Manager) IsConnectedTo(channelID string) bool {
	return m.operations.GetConnection().IsConnectedTo(channelID)
}

func (m *Manager) Shutdown(ctx context.Context) error {
	logger.Info.Println("Shutting down voice manager...")
	return m.operations.GetConnection().Shutdown(ctx)
}

func (m *Manager) Name() string {
	return "VoiceManager"
}
