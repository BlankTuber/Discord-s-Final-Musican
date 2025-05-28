package voice

import (
	"context"
	"fmt"
	"musicbot/internal/logger"
	"musicbot/internal/state"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	maxJoinRetries = 3
	joinRetryDelay = 2 * time.Second
)

type Connection struct {
	session      *discordgo.Session
	stateManager *state.Manager
	connection   *discordgo.VoiceConnection
}

func NewConnection(session *discordgo.Session, stateManager *state.Manager) *Connection {
	return &Connection{
		session:      session,
		stateManager: stateManager,
	}
}

func (c *Connection) Join(guildID, channelID string) error {
	if c.stateManager.IsShuttingDown() {
		logger.Debug.Println("Ignoring join request during shutdown")
		return fmt.Errorf("bot is shutting down")
	}

	if c.stateManager.IsOperationInProgress() {
		return fmt.Errorf("operation already in progress")
	}

	c.stateManager.SetJoining(true)
	defer c.stateManager.SetJoining(false)

	if c.connection != nil && c.connection.ChannelID == channelID {
		logger.Info.Printf("Already connected to channel %s", channelID)
		return nil
	}

	if c.connection != nil {
		logger.Info.Println("Disconnecting from current channel...")
		c.connection.Disconnect()
		c.connection = nil
		time.Sleep(500 * time.Millisecond)
	}

	var lastErr error
	for attempt := 1; attempt <= maxJoinRetries; attempt++ {
		if c.stateManager.IsShuttingDown() {
			logger.Debug.Printf("Aborting join attempt %d due to shutdown", attempt)
			return fmt.Errorf("bot is shutting down")
		}

		logger.Info.Printf("Joining voice channel %s (attempt %d/%d)", channelID, attempt, maxJoinRetries)

		vc, err := c.session.ChannelVoiceJoin(guildID, channelID, false, true)
		if err != nil {
			lastErr = err
			logger.Error.Printf("Join attempt %d failed: %v", attempt, err)

			if attempt < maxJoinRetries && !c.stateManager.IsShuttingDown() {
				time.Sleep(joinRetryDelay * time.Duration(attempt))
				continue
			}
		} else {
			c.connection = vc
			c.stateManager.SetCurrentChannel(channelID)
			c.stateManager.SetConnected(true)

			logger.Info.Printf("Successfully joined voice channel %s", channelID)
			time.Sleep(300 * time.Millisecond)
			return nil
		}
	}

	return fmt.Errorf("failed to join voice channel after %d attempts: %w", maxJoinRetries, lastErr)
}

func (c *Connection) Leave() error {
	if c.stateManager.IsOperationInProgress() && !c.stateManager.IsShuttingDown() {
		return fmt.Errorf("operation already in progress")
	}

	if !c.stateManager.IsShuttingDown() {
		c.stateManager.SetLeaving(true)
		defer c.stateManager.SetLeaving(false)
	}

	if c.connection == nil {
		logger.Info.Println("Already disconnected from voice")
		return nil
	}

	channelID := c.connection.ChannelID
	logger.Info.Printf("Leaving voice channel %s", channelID)

	err := c.connection.Disconnect()
	c.connection = nil
	c.stateManager.SetCurrentChannel("")
	c.stateManager.SetConnected(false)

	if err != nil {
		logger.Error.Printf("Error disconnecting from voice: %v", err)
	} else {
		logger.Info.Println("Successfully disconnected from voice")
	}

	return err
}

func (c *Connection) GetConnection() *discordgo.VoiceConnection {
	return c.connection
}

func (c *Connection) IsConnectedTo(channelID string) bool {
	if c.connection == nil {
		return false
	}
	return c.connection.ChannelID == channelID
}

func (c *Connection) HandleDisconnect() {
	if c.stateManager.IsShuttingDown() {
		logger.Info.Println("Expected voice disconnection during shutdown")
	} else {
		logger.Info.Println("Handling unexpected voice disconnection")
	}

	c.connection = nil
	c.stateManager.SetCurrentChannel("")
	c.stateManager.SetConnected(false)
}

func (c *Connection) Shutdown(ctx context.Context) error {
	logger.Info.Println("Shutting down voice connection...")

	c.stateManager.SetShuttingDown(true)

	if c.connection != nil {
		err := c.connection.Disconnect()
		c.connection = nil
		c.stateManager.SetCurrentChannel("")
		c.stateManager.SetConnected(false)
		return err
	}

	return nil
}

func (c *Connection) Name() string {
	return "VoiceConnection"
}
