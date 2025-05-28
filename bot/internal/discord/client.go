package discord

import (
	"context"
	"fmt"
	"time"

	"musicbot/internal/config"
	"musicbot/internal/discord/commands"
	"musicbot/internal/logger"
	"musicbot/internal/radio"
	"musicbot/internal/state"
	"musicbot/internal/voice"

	"github.com/bwmarrin/discordgo"
)

type Client struct {
	session       *discordgo.Session
	stateManager  *state.Manager
	voiceManager  *voice.Manager
	radioManager  *radio.Manager
	commandRouter *commands.Router
	eventHandler  *EventHandler
	dbManager     *config.DatabaseManager
}

func NewClient(token string, stateManager *state.Manager, dbManager *config.DatabaseManager) (*Client, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}

	session.Identify.Intents = discordgo.IntentsGuildVoiceStates | discordgo.IntentsGuilds

	voiceManager := voice.NewManager(session, stateManager)
	radioManager := radio.NewManager(stateManager, config.GetDefaultStreams())
	commandRouter := commands.NewRouter(session)
	eventHandler := NewEventHandler(session, voiceManager, radioManager, stateManager)

	client := &Client{
		session:       session,
		stateManager:  stateManager,
		voiceManager:  voiceManager,
		radioManager:  radioManager,
		commandRouter: commandRouter,
		eventHandler:  eventHandler,
		dbManager:     dbManager,
	}

	client.registerCommands()
	client.registerEventHandlers()

	return client, nil
}

func (c *Client) Connect() error {
	logger.Info.Println("Connecting to Discord...")

	err := c.session.Open()
	if err != nil {
		return fmt.Errorf("failed to connect to Discord: %w", err)
	}

	logger.Info.Println("Connected to Discord")
	return nil
}

func (c *Client) Disconnect() error {
	logger.Info.Println("Disconnecting from Discord...")
	return c.session.Close()
}

func (c *Client) UpdateCommands() error {
	logger.Info.Println("Updating slash commands...")
	return c.commandRouter.UpdateCommands()
}

func (c *Client) StartIdleMode(guildID string) error {
	logger.Info.Println("Starting idle mode...")

	err := c.voiceManager.ReturnToIdle(guildID)
	if err != nil {
		return fmt.Errorf("failed to join idle channel: %w", err)
	}

	c.stateManager.SetBotState(state.StateIdle)

	time.Sleep(500 * time.Millisecond)

	vc := c.voiceManager.GetVoiceConnection()
	if vc != nil {
		err = c.radioManager.Start(vc)
		if err != nil {
			logger.Error.Printf("Failed to start radio: %v", err)
		}
	}

	logger.Info.Println("Idle mode started successfully")
	return nil
}

func (c *Client) Shutdown(ctx context.Context) error {
	logger.Info.Println("Shutting down Discord client...")

	c.radioManager.Stop()

	time.Sleep(500 * time.Millisecond)

	if err := c.voiceManager.Shutdown(ctx); err != nil {
		logger.Error.Printf("Error shutting down voice manager: %v", err)
	}

	if err := c.session.Close(); err != nil {
		logger.Error.Printf("Error closing Discord session: %v", err)
		return err
	}

	logger.Info.Println("Discord client shut down successfully")
	return nil
}

func (c *Client) Name() string {
	return "DiscordClient"
}

func (c *Client) GetVoiceManager() *voice.Manager {
	return c.voiceManager
}

func (c *Client) GetRadioManager() *radio.Manager {
	return c.radioManager
}

func (c *Client) registerCommands() {
	c.commandRouter.Register(commands.NewJoinCommand(c.voiceManager, c.radioManager, c.stateManager))
	c.commandRouter.Register(commands.NewLeaveCommand(c.voiceManager, c.radioManager, c.stateManager))
	c.commandRouter.Register(commands.NewChangeStreamCommand(c.voiceManager, c.radioManager, c.dbManager))
}

func (c *Client) registerEventHandlers() {
	c.session.AddHandler(c.eventHandler.HandleReady)
	c.session.AddHandler(c.eventHandler.HandleVoiceStateUpdate)
	c.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		c.commandRouter.Handle(i)
	})
}
