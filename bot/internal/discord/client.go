package discord

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/config"
	"quidque.com/discord-musican/internal/database"
	"quidque.com/discord-musican/internal/downloader"
	"quidque.com/discord-musican/internal/logger"
	"quidque.com/discord-musican/internal/queue"
)

var (
	disablingIdleModeMu sync.Mutex
	disablingIdleMode   bool
)

type Client struct {
	Token    string
	ClientID string
	Session  *discordgo.Session
	Router   *CommandRouter

	VoiceManager     *VoiceManager
	RadioManager     *RadioManager
	QueueManager     *queue.Manager
	DownloaderClient *downloader.Client
	DBManager        *database.Manager

	DefaultGuildID   string
	DefaultVCID      string
	IdleTimeout      int
	LastActivityTime time.Time
	IsInIdleMode     bool
	IdleModeDisabled bool
	IdleCheckTicker  *time.Ticker

	SearchResultsCache map[string][]*audio.Track

	CommandsEnabled bool
	Mu              sync.RWMutex
	stopChan        chan struct{}
}

func NewClient(cfg config.Config) (*Client, error) {
	if cfg.DISCORD_TOKEN == "" {
		return nil, errors.New("discord token is required")
	}

	if cfg.CLIENT_ID == "" {
		return nil, errors.New("client ID is required")
	}

	if cfg.UDS_PATH == "" {
		cfg.UDS_PATH = "/tmp/downloader.sock"
	}

	client := &Client{
		Token:              cfg.DISCORD_TOKEN,
		ClientID:           cfg.CLIENT_ID,
		DefaultGuildID:     cfg.DEFAULT_GUILD_ID,
		DefaultVCID:        cfg.DEFAULT_VC_ID,
		IdleTimeout:        cfg.IDLE_TIMEOUT,
		LastActivityTime:   time.Now(),
		IsInIdleMode:       false,
		IdleModeDisabled:   false,
		SearchResultsCache: make(map[string][]*audio.Track),
		CommandsEnabled:    true,
		stopChan:           make(chan struct{}),
	}

	// Initialize the Discord session
	session, err := discordgo.New("Bot " + cfg.DISCORD_TOKEN)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}
	client.Session = session

	// Initialize the downloader client
	client.DownloaderClient = downloader.NewClient(cfg.UDS_PATH)

	// Initialize the command router
	client.Router = NewCommandRouter(client)

	// Initialize the database manager
	dbManager, err := database.NewManager(cfg.DB_PATH)
	if err != nil {
		logger.WarnLogger.Printf("Failed to connect to database: %v", err)
		logger.WarnLogger.Println("Database features will be disabled")
	} else {
		logger.InfoLogger.Println("Successfully connected to database")
		client.DBManager = dbManager
	}

	// Initialize the queue manager
	client.QueueManager = queue.NewManager(client.DBManager)
	client.QueueManager.SetEventCallback(client.handleQueueEvent)

	// Initialize the voice manager
	client.VoiceManager = NewVoiceManager(client)

	// Initialize the radio manager
	client.RadioManager = NewRadioManager(client, cfg.RADIO_URL, cfg.VOLUME)

	// Setup handlers
	session.AddHandler(client.handleReady)
	session.AddHandler(client.handleVoiceStateUpdate)
	session.AddHandler(client.handleInteraction)
	session.AddHandler(client.handleMessageCreate)

	session.Identify.Intents = discordgo.IntentsGuildVoiceStates |
		discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	return client, nil
}

// Downloader client will handle events from the downloader service
func (c *Client) handleDownloaderEvent(eventType string, data map[string]any) {
	switch eventType {
	case "playlist_item_downloaded":
		// When a playlist item is downloaded, add it to the queue
		if trackData, ok := data["track"].(map[string]any); ok {
			guildID, _ := data["guild_id"].(string)
			if guildID == "" {
				return
			}

			track := &audio.Track{}

			if title, ok := trackData["title"].(string); ok {
				track.Title = title
			}

			if url, ok := trackData["url"].(string); ok {
				track.URL = url
			}

			if filePath, ok := trackData["file_path"].(string); ok {
				track.FilePath = filePath
			}

			if duration, ok := trackData["duration"].(float64); ok {
				track.Duration = int(duration)
			}

			if artist, ok := trackData["artist"].(string); ok {
				track.ArtistName = artist
			}

			if thumbnail, ok := trackData["thumbnail_url"].(string); ok {
				track.ThumbnailURL = thumbnail
			}

			if requester, ok := data["requester"].(string); ok {
				track.Requester = requester
			}

			track.RequestedAt = time.Now().Unix()

			c.QueueManager.AddTrack(guildID, track)
		}
	}
}

func (c *Client) handleQueueEvent(event queue.QueueEvent) {
	switch event.Type {
	case queue.EventTrackAdded:
		if c.QueueManager.GetQueueLength(event.GuildID) == 1 && event.Track != nil {
			logger.InfoLogger.Printf("First track added to queue, checking if should start playback")

			playerState := c.VoiceManager.GetPlayerState(event.GuildID)
			if playerState != audio.StatePlaying && playerState != audio.StatePaused {
				if c.VoiceManager.IsConnected(event.GuildID) {
					logger.InfoLogger.Printf("Starting playback for first track in queue")
					c.StartPlayback(event.GuildID)
				} else {
					logger.InfoLogger.Printf("Not connected to voice, waiting for manual start")
				}
			}
		}

	case queue.EventTracksAdded:
		if len(event.Tracks) > 0 && c.QueueManager.GetQueueLength(event.GuildID) == len(event.Tracks) {
			logger.InfoLogger.Printf("First tracks added to queue, checking if should start playback")

			playerState := c.VoiceManager.GetPlayerState(event.GuildID)
			if playerState != audio.StatePlaying && playerState != audio.StatePaused {
				if c.VoiceManager.IsConnected(event.GuildID) {
					logger.InfoLogger.Printf("Starting playback for first tracks in queue")
					c.StartPlayback(event.GuildID)
				} else {
					logger.InfoLogger.Printf("Not connected to voice, waiting for manual start")
				}
			}
		}
	}
}

func (c *Client) StartPlayback(guildID string) {
	logger.InfoLogger.Printf("StartPlayback called for guild %s", guildID)

	c.DisableIdleMode()

	if !c.VoiceManager.IsConnected(guildID) {
		logger.WarnLogger.Printf("Voice connection not found for guild %s", guildID)
		return
	}

	if c.RadioManager.IsPlaying() {
		c.RadioManager.Stop()
		time.Sleep(300 * time.Millisecond)
	}

	logger.InfoLogger.Printf("Starting playback from queue in guild %s", guildID)
	c.VoiceManager.StartPlayingFromQueue(guildID)
}

func (c *Client) JoinVoiceChannel(guildID, channelID string) error {
	c.StartActivity()

	c.VoiceManager.mu.Lock()
	c.VoiceManager.lastKnownChannels[guildID] = channelID
	c.VoiceManager.mu.Unlock()

	err := c.VoiceManager.JoinChannel(guildID, channelID)
	if err != nil {
		logger.ErrorLogger.Printf("Failed to join voice channel %s in guild %s: %v", channelID, guildID, err)
		return err
	}

	logger.InfoLogger.Printf("Successfully joined voice channel %s in guild %s", channelID, guildID)

	return nil
}

func (c *Client) RobustJoinVoiceChannel(guildID, channelID string) error {
	var lastError error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}

		c.DisableIdleMode()

		err := c.JoinVoiceChannel(guildID, channelID)
		if err == nil {
			time.Sleep(300 * time.Millisecond)
			if c.VoiceManager.IsConnectedToChannel(guildID, channelID) {
				logger.InfoLogger.Printf("Successfully joined voice channel after %d attempt(s)", attempt+1)
				return nil
			}
			lastError = fmt.Errorf("connection verification failed")
		} else {
			lastError = err
		}

		logger.WarnLogger.Printf("Join attempt %d failed: %v", attempt+1, lastError)
	}

	return fmt.Errorf("failed to join voice channel after multiple attempts: %w", lastError)
}

func (c *Client) Connect() error {
	// Connect to Discord
	err := c.Session.Open()
	if err != nil {
		return fmt.Errorf("failed to connect to Discord: %w", err)
	}

	// Connect to the downloader service
	err = c.DownloaderClient.Connect()
	if err != nil {
		logger.WarnLogger.Printf("Failed to connect to downloader service: %v", err)
		logger.WarnLogger.Println("Make sure the downloader service is running!")
	} else {
		logger.InfoLogger.Println("Successfully connected to downloader service")

		// Set up event handling from downloader
		c.DownloaderClient.SetEventCallback(c.handleDownloaderEvent)
	}

	// Start the idle checker
	c.startIdleChecker()

	// Refresh slash commands
	err = c.Router.RefreshSlashCommands()
	if err != nil {
		logger.ErrorLogger.Printf("Failed to refresh slash commands: %v", err)
	}

	// Schedule idle mode startup
	go func() {
		logger.InfoLogger.Println("Scheduling idle mode startup in 3 seconds...")
		time.Sleep(3 * time.Second)
		c.startIdleMode()
	}()

	return nil
}

func (c *Client) Disconnect() error {
	// Stop the idle checker
	if c.IdleCheckTicker != nil {
		c.IdleCheckTicker.Stop()
	}

	// Stop the radio
	c.RadioManager.Stop()

	// Close the stop channel
	close(c.stopChan)

	// Disconnect from all voice channels
	c.VoiceManager.DisconnectAll()

	// Close the database connection
	if c.DBManager != nil {
		if err := c.DBManager.Close(); err != nil {
			logger.ErrorLogger.Printf("Error closing database: %v", err)
		}
	}

	// Disconnect from the downloader service
	if c.DownloaderClient != nil {
		if err := c.DownloaderClient.Disconnect(); err != nil {
			logger.ErrorLogger.Printf("Error disconnecting from downloader service: %v", err)
		}
	}

	// Close the Discord session
	return c.Session.Close()
}

func (c *Client) DisableCommands() {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.CommandsEnabled = false
}

func (c *Client) IsCommandsEnabled() bool {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.CommandsEnabled
}

func (c *Client) StartActivity() {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.LastActivityTime = time.Now()
}

func (c *Client) GetUserVoiceChannel(guildID, userID string) (string, error) {
	vs, err := c.Session.State.VoiceState(guildID, userID)
	if err != nil || vs == nil || vs.ChannelID == "" {
		return "", errors.New("user is not in a voice channel")
	}

	return vs.ChannelID, nil
}

func (c *Client) LeaveVoiceChannel(guildID string) error {
	return c.VoiceManager.LeaveChannel(guildID)
}

func (c *Client) SetDefaultVoiceChannel(guildID, channelID string) {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	c.DefaultGuildID = guildID
	c.DefaultVCID = channelID

	logger.InfoLogger.Printf("Default voice channel set to %s in guild %s", channelID, guildID)
}

func (c *Client) checkChannelEmpty(guildID, channelID string) bool {
	guild, err := c.Session.State.Guild(guildID)
	if err != nil {
		logger.ErrorLogger.Printf("Error getting guild %s: %v", guildID, err)
		return false
	}

	botID := c.Session.State.User.ID
	userCount := 0

	for _, vs := range guild.VoiceStates {
		if vs.ChannelID == channelID {
			if vs.UserID != botID {
				userCount++
			}
		}
	}

	return userCount == 0
}

func (c *Client) startIdleMode() {
	c.Mu.Lock()

	if c.IsInIdleMode {
		logger.InfoLogger.Println("Idle mode is already active")
		c.Mu.Unlock()
		return
	}

	if c.DefaultGuildID == "" || c.DefaultVCID == "" {
		logger.WarnLogger.Println("Cannot enter idle mode: default voice channel not configured")
		c.Mu.Unlock()
		return
	}

	if c.IdleModeDisabled {
		logger.InfoLogger.Println("Idle mode is currently disabled")
		c.Mu.Unlock()
		return
	}

	for guildID := range c.VoiceManager.GetConnectedChannels() {
		queueLength := c.QueueManager.GetQueueLength(guildID)
		playerState := c.VoiceManager.GetPlayerState(guildID)

		if queueLength > 0 || playerState == audio.StatePlaying || playerState == audio.StatePaused {
			logger.InfoLogger.Println("Active playback or queued tracks exist, not entering idle mode")
			c.Mu.Unlock()
			return
		}
	}

	c.IsInIdleMode = true
	defaultGuildID := c.DefaultGuildID
	defaultVCID := c.DefaultVCID
	c.Mu.Unlock()

	logger.InfoLogger.Println("Entering idle mode")

	c.VoiceManager.StopAllPlayback()

	if c.VoiceManager.IsConnectedToChannel(defaultGuildID, defaultVCID) {
		logger.InfoLogger.Println("Already in the default voice channel, staying connected")
	} else {
		err := c.JoinVoiceChannel(defaultGuildID, defaultVCID)
		if err != nil {
			logger.ErrorLogger.Printf("Failed to join default voice channel: %v", err)

			c.Mu.Lock()
			c.IsInIdleMode = false
			c.Mu.Unlock()
			return
		}
	}

	time.Sleep(500 * time.Millisecond)
	c.RadioManager.Start()

	c.Session.UpdateGameStatus(0, "Radio Mode | Use /help")
}

func (c *Client) startIdleChecker() {
	c.IdleCheckTicker = time.NewTicker(30 * time.Second)

	go func() {
		for {
			select {
			case <-c.stopChan:
				return
			case <-c.IdleCheckTicker.C:
				c.checkIdleState()
			}
		}
	}()
}

func (c *Client) checkIdleState() {
	c.Mu.RLock()
	lastActivity := c.LastActivityTime
	isInIdleMode := c.IsInIdleMode
	idleTimeout := c.IdleTimeout
	idleModeDisabled := c.IdleModeDisabled
	c.Mu.RUnlock()

	if isInIdleMode || idleModeDisabled {
		return
	}

	timeSinceActivity := time.Since(lastActivity)

	for guildID, channelID := range c.VoiceManager.GetConnectedChannels() {
		queueLength := c.QueueManager.GetQueueLength(guildID)
		playerState := c.VoiceManager.GetPlayerState(guildID)

		if queueLength > 0 || playerState == audio.StatePlaying || playerState == audio.StatePaused {
			return
		}

		if c.checkChannelEmpty(guildID, channelID) {
			logger.InfoLogger.Println("Voice channel is empty, checking if we should enter idle mode")

			c.Mu.RLock()
			defaultGuildID := c.DefaultGuildID
			defaultVCID := c.DefaultVCID
			c.Mu.RUnlock()

			if guildID == defaultGuildID && channelID == defaultVCID {
				logger.InfoLogger.Println("Already in the idle channel, enabling idle mode")
				c.startIdleMode()
			} else {
				logger.InfoLogger.Println("In non-idle channel that's empty, moving to idle channel")
				c.LeaveVoiceChannel(guildID)

				go func() {
					time.Sleep(2 * time.Second)
					c.startIdleMode()
				}()
			}
			return
		}
	}

	if timeSinceActivity.Seconds() > float64(idleTimeout) {
		logger.InfoLogger.Printf("Bot idle for %v seconds, entering radio mode", int(timeSinceActivity.Seconds()))
		c.startIdleMode()
	}
}

func (c *Client) EnableIdleMode() {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	c.IdleModeDisabled = false
	logger.InfoLogger.Println("Idle mode enabled")
}

func (c *Client) DisableIdleMode() {
	c.Mu.Lock()
	c.IdleModeDisabled = true
	wasInIdleMode := c.IsInIdleMode
	c.IsInIdleMode = false
	c.LastActivityTime = time.Now()
	c.Mu.Unlock()

	if wasInIdleMode && c.RadioManager.IsPlaying() {
		logger.InfoLogger.Println("Stopping radio stream due to idle mode being disabled")
		c.RadioManager.Stop()
		time.Sleep(300 * time.Millisecond)
	}

	logger.InfoLogger.Println("Idle mode disabled")
}

func (c *Client) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	logger.InfoLogger.Printf("Logged in as: %s#%s", s.State.User.Username, s.State.User.Discriminator)
	logger.InfoLogger.Printf("Bot is in %d servers", len(r.Guilds))

	s.UpdateGameStatus(0, "Radio Mode | Use /help")
}

func (c *Client) handleVoiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	if v.UserID == s.State.User.ID {
		if v.ChannelID == "" {
			logger.InfoLogger.Printf("Bot was disconnected from voice in guild %s", v.GuildID)

			c.Mu.Lock()
			wasInIdleMode := c.IsInIdleMode
			c.IsInIdleMode = false
			c.Mu.Unlock()

			if wasInIdleMode {
				c.RadioManager.Stop()
			}

			c.VoiceManager.HandleDisconnect(v.GuildID)

			if c.QueueManager.GetQueueLength(v.GuildID) > 0 {
				logger.InfoLogger.Printf("Queue has tracks, will wait for manual reconnect")
				return
			}

			c.Mu.RLock()
			idleModeDisabled := c.IdleModeDisabled
			c.Mu.RUnlock()

			if !idleModeDisabled {
				go func() {
					time.Sleep(5 * time.Second)
					c.startIdleMode()
				}()
			}

			return
		}

		currentChannel := c.VoiceManager.GetConnectedChannel(v.GuildID)
		if currentChannel != "" && currentChannel != v.ChannelID {
			logger.InfoLogger.Printf("Bot was moved from channel %s to channel %s",
				currentChannel, v.ChannelID)

			c.Mu.Lock()
			isInIdleChannel := (v.ChannelID == c.DefaultVCID && v.GuildID == c.DefaultGuildID)
			wasInIdleMode := c.IsInIdleMode

			if isInIdleChannel && !c.IdleModeDisabled {
				c.IsInIdleMode = true
				logger.InfoLogger.Println("Bot was moved to idle channel, enabling idle mode")
			} else if wasInIdleMode {
				c.IsInIdleMode = false
				logger.InfoLogger.Println("Bot was moved out of idle channel, disabling idle mode")
			}
			c.Mu.Unlock()

			c.VoiceManager.HandleChannelMove(v.GuildID, v.ChannelID)

			if !wasInIdleMode && isInIdleChannel && !c.IdleModeDisabled {
				go c.RadioManager.Start()
			} else if wasInIdleMode && !isInIdleChannel {
				c.RadioManager.Stop()
			}

			return
		}
	}

	if v.UserID != s.State.User.ID {
		for guildID, channelID := range c.VoiceManager.GetConnectedChannels() {
			if c.checkChannelEmpty(guildID, channelID) {
				logger.InfoLogger.Println("Bot is alone in voice channel")

				queueLength := c.QueueManager.GetQueueLength(guildID)
				playerState := c.VoiceManager.GetPlayerState(guildID)

				if playerState == audio.StatePlaying || playerState == audio.StatePaused || queueLength > 0 {
					logger.InfoLogger.Println("Stopping playback because bot is alone")
					c.VoiceManager.StopAllPlayback()
					c.Session.UpdateGameStatus(0, "Queue is empty | Use /play")
				}

				c.Mu.Lock()
				defaultGuildID := c.DefaultGuildID
				defaultVCID := c.DefaultVCID
				idleModeDisabled := c.IdleModeDisabled
				c.Mu.Unlock()

				if channelID != defaultVCID || guildID != defaultGuildID {
					c.LeaveVoiceChannel(guildID)

					if !idleModeDisabled {
						go func() {
							time.Sleep(2 * time.Second)
							c.startIdleMode()
						}()
					}
				} else if !idleModeDisabled {
					c.startIdleMode()
				}

				break
			}
		}
	}
}

func (c *Client) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	c.Router.HandleInteraction(s, i)
}

func (c *Client) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	c.StartActivity()
}

type ShutdownManager struct {
	client *Client
	wg     sync.WaitGroup
}

func NewShutdownManager(client *Client) *ShutdownManager {
	return &ShutdownManager{
		client: client,
	}
}

func (sm *ShutdownManager) Shutdown(ctx context.Context) error {
	logger.InfoLogger.Println("Initiating graceful shutdown...")

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	sm.client.DisableCommands()

	if sm.client.IdleCheckTicker != nil {
		sm.client.IdleCheckTicker.Stop()
	}

	if sm.client.RadioManager != nil {
		sm.client.RadioManager.Stop()
	}

	// Use waitgroup for synchronization
	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()
		sm.client.VoiceManager.StopAllPlayback()
	}()

	// Disconnect voice channels
	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()
		sm.client.VoiceManager.DisconnectAll()
	}()

	// Wait for cleanup with timeout
	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		logger.WarnLogger.Println("Shutdown timed out, forcing exit...")
		break
	case <-done:
		logger.InfoLogger.Println("All components shut down successfully")
	}

	// Close DB and Discord session
	if sm.client.DBManager != nil {
		if err := sm.client.DBManager.Close(); err != nil {
			logger.ErrorLogger.Printf("Error closing database: %v", err)
		}
	}

	// Disconnect from downloader service
	if sm.client.DownloaderClient != nil {
		if err := sm.client.DownloaderClient.Disconnect(); err != nil {
			logger.ErrorLogger.Printf("Error disconnecting from downloader: %v", err)
		}
	}

	// Close Discord session
	err := sm.client.Session.Close()
	if err != nil {
		logger.ErrorLogger.Printf("Error closing Discord session: %v", err)
	}

	return err
}
