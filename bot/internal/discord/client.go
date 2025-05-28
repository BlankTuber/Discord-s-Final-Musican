package discord

import (
	"context"
	"fmt"
	"time"

	"musicbot/internal/config"
	"musicbot/internal/discord/commands"
	"musicbot/internal/logger"
	"musicbot/internal/music"
	"musicbot/internal/radio"
	"musicbot/internal/socket"
	"musicbot/internal/state"
	"musicbot/internal/voice"

	"github.com/bwmarrin/discordgo"
)

type Client struct {
	session       *discordgo.Session
	stateManager  *state.Manager
	voiceManager  *voice.Manager
	radioManager  *radio.Manager
	musicManager  *music.Manager
	commandRouter *commands.Router
	eventHandler  *EventHandler
	dbManager     *config.DatabaseManager
	socketClient  *socket.Client
	searchCommand *commands.SearchCommand
}

func NewClient(token string, stateManager *state.Manager, dbManager *config.DatabaseManager, socketClient *socket.Client) (*Client, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}

	session.Identify.Intents = discordgo.IntentsGuildVoiceStates | discordgo.IntentsGuilds

	voiceManager := voice.NewManager(session, stateManager)
	radioManager := radio.NewManager(stateManager, config.GetDefaultStreams())
	musicManager := music.NewManager(stateManager, dbManager, radioManager, socketClient)
	commandRouter := commands.NewRouter(session)
	eventHandler := NewEventHandler(session, voiceManager, radioManager, musicManager, stateManager)

	client := &Client{
		session:       session,
		stateManager:  stateManager,
		voiceManager:  voiceManager,
		radioManager:  radioManager,
		musicManager:  musicManager,
		commandRouter: commandRouter,
		eventHandler:  eventHandler,
		dbManager:     dbManager,
		socketClient:  socketClient,
	}

	client.setupMusicManager()
	client.registerCommands()
	client.registerEventHandlers()

	return client, nil
}

func (c *Client) setupMusicManager() {
	c.musicManager.SetVoiceConnectionGetter(c.voiceManager.GetVoiceConnection)

	if c.socketClient != nil {
		c.socketClient.SetDownloadHandler(func(song *state.Song) {
			err := c.musicManager.OnDownloadComplete(song)
			if err != nil {
				logger.Error.Printf("Failed to handle download completion: %v", err)
			}
		})

		c.socketClient.SetPlaylistHandler(func(songs []state.Song) {
			for _, song := range songs {
				err := c.musicManager.OnDownloadComplete(&song)
				if err != nil {
					logger.Error.Printf("Failed to handle playlist song: %v", err)
				}
			}
		})
	}
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

	c.musicManager.Stop()
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

func (c *Client) GetMusicManager() *music.Manager {
	return c.musicManager
}

func (c *Client) registerCommands() {
	c.commandRouter.Register(commands.NewJoinCommand(c.voiceManager, c.radioManager, c.musicManager, c.stateManager))
	c.commandRouter.Register(commands.NewLeaveCommand(c.voiceManager, c.radioManager, c.musicManager, c.stateManager))
	c.commandRouter.Register(commands.NewChangeStreamCommand(c.voiceManager, c.radioManager, c.dbManager))
	c.commandRouter.Register(commands.NewPlayCommand(c.voiceManager, c.radioManager, c.musicManager, c.stateManager))
	c.commandRouter.Register(commands.NewPlaylistCommand(c.voiceManager, c.radioManager, c.musicManager, c.stateManager))
	c.commandRouter.Register(commands.NewQueueCommand(c.musicManager, c.stateManager))
	c.commandRouter.Register(commands.NewSkipCommand(c.musicManager, c.stateManager))
	c.commandRouter.Register(commands.NewNowPlayingCommand(c.musicManager, c.radioManager, c.stateManager))
	c.commandRouter.Register(commands.NewClearCommand(c.musicManager, c.stateManager))

	c.searchCommand = commands.NewSearchCommand(c.voiceManager, c.radioManager, c.musicManager, c.stateManager, c.socketClient)
	c.commandRouter.Register(c.searchCommand)
}

func (c *Client) registerEventHandlers() {
	c.session.AddHandler(c.eventHandler.HandleReady)
	c.session.AddHandler(c.eventHandler.HandleVoiceStateUpdate)
	c.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type == discordgo.InteractionApplicationCommand {
			c.commandRouter.Handle(i)
		} else if i.Type == discordgo.InteractionMessageComponent {
			c.handleMessageComponent(s, i)
		}
	})
}

func (c *Client) handleMessageComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if len(customID) > 13 && customID[:13] == "search_select" {
		if c.searchCommand != nil {
			err := c.searchCommand.HandleSearchSelection(s, i)
			if err != nil {
				logger.Error.Printf("Search selection error: %v", err)
			}
		}
	}
}
