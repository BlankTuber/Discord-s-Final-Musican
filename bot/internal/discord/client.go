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
		// Start playback if this is the first track
		if c.QueueManager.GetQueueLength(event.GuildID) == 1 {
			c.StartPlayback(event.GuildID)
		}

	case queue.EventTracksAdded:
		// Start playback if these are the first tracks
		if len(event.Tracks) > 0 && c.QueueManager.GetQueueLength(event.GuildID) == len(event.Tracks) {
			c.StartPlayback(event.GuildID)
		}
	}
}

func (c *Client) StartPlayback(guildID string) {
	logger.InfoLogger.Printf("StartPlayback called for guild %s", guildID)

	// Aggressively disable idle mode
	c.DisableIdleMode()

	// Make sure we're connected to voice before starting
	if !c.VoiceManager.IsConnected(guildID) {
		logger.WarnLogger.Printf("Voice connection not found for guild %s, attempting to reconnect", guildID)

		// Try to find a valid channel ID to connect to
		var channelID string

		// First try to use the voice channel the bot was last in
		if lastChannel := c.VoiceManager.GetLastKnownChannel(guildID); lastChannel != "" {
			channelID = lastChannel
			logger.InfoLogger.Printf("Using last known channel: %s", channelID)
		} else {
			// Otherwise check where guild members are
			guild, err := c.Session.State.Guild(guildID)
			if err != nil {
				// If not in state, try fetching from API
				guild, err = c.Session.Guild(guildID)
				if err != nil {
					logger.ErrorLogger.Printf("Failed to get guild info: %v", err)
				}
			}

			if guild != nil {
				// Look through voice states to find a channel with users
				for _, vs := range guild.VoiceStates {
					if vs.UserID != c.Session.State.User.ID && vs.ChannelID != "" {
						channelID = vs.ChannelID
						logger.InfoLogger.Printf("Found channel with users: %s", channelID)
						break
					}
				}
			}

			// If still no channel found, fall back to default VC if available
			if channelID == "" && c.DefaultVCID != "" && c.DefaultGuildID == guildID {
				channelID = c.DefaultVCID
				logger.InfoLogger.Printf("Falling back to default voice channel: %s", channelID)
			}
		}

		if channelID == "" {
			logger.ErrorLogger.Printf("Failed to find a voice channel to join in guild %s", guildID)
			return
		}

		// Try to join with retries
		var joinSuccess bool
		for attempt := 0; attempt < 3; attempt++ {
			err := c.RobustJoinVoiceChannel(guildID, channelID)
			if err == nil {
				joinSuccess = true
				logger.InfoLogger.Printf("Successfully reconnected to voice channel %s", channelID)

				// Wait a moment for the connection to stabilize
				time.Sleep(500 * time.Millisecond)
				break
			}
			logger.WarnLogger.Printf("Failed to join voice channel (attempt %d): %v", attempt+1, err)
			time.Sleep(500 * time.Millisecond)
		}

		if !joinSuccess {
			logger.ErrorLogger.Printf("Failed to reconnect to voice channel after multiple attempts")
			return
		}
	}

	// Stop the radio if it's playing
	if c.IsInIdleMode {
		c.RadioManager.Stop()
		c.IsInIdleMode = false
		logger.InfoLogger.Println("Radio stopped for track playback")
		time.Sleep(300 * time.Millisecond) // Wait for radio to stop
	}

	// Start playing from the queue
	logger.InfoLogger.Printf("Starting playback from queue in guild %s", guildID)
	c.VoiceManager.StartPlayingFromQueue(guildID)
}

// JoinVoiceChannel joins a voice channel in a guild
func (c *Client) JoinVoiceChannel(guildID, channelID string) error {
	// Use VoiceManager's JoinChannel function
	err := c.VoiceManager.JoinChannel(guildID, channelID)
	if err == nil {
		// Set last activity time
		c.StartActivity()

		// Store the channel in last known channels map
		c.VoiceManager.mu.Lock()
		c.VoiceManager.lastKnownChannels[guildID] = channelID
		c.VoiceManager.mu.Unlock()
	}
	return err
}

// RobustJoinVoiceChannel tries multiple times to join a voice channel
func (c *Client) RobustJoinVoiceChannel(guildID, channelID string) error {
	// Try joining up to 3 times
	var lastError error
	for attempt := 0; attempt < 3; attempt++ {
		// Call JoinVoiceChannel
		err := c.JoinVoiceChannel(guildID, channelID)
		if err == nil {
			// Verify the connection succeeded
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
		time.Sleep(500 * time.Millisecond)
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

	c.IsInIdleMode = true
	defaultGuildID := c.DefaultGuildID
	defaultVCID := c.DefaultVCID
	c.Mu.Unlock()

	logger.InfoLogger.Println("Entering idle mode")

	// Stop all playback
	c.VoiceManager.StopAllPlayback()

	// Join the default voice channel
	if c.VoiceManager.IsConnectedToChannel(defaultGuildID, defaultVCID) {
		logger.InfoLogger.Println("Already in the default voice channel, staying connected")
	} else {
		err := c.RobustJoinVoiceChannel(defaultGuildID, defaultVCID)
		if err != nil {
			logger.ErrorLogger.Printf("Failed to join default voice channel: %v", err)

			c.Mu.Lock()
			c.IsInIdleMode = false
			c.Mu.Unlock()
			return
		}
	}

	// Start the radio
	time.Sleep(250 * time.Millisecond)
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
	defaultGuildID := c.DefaultGuildID
	defaultVCID := c.DefaultVCID
	c.Mu.RUnlock()

	if isInIdleMode {
		return
	}

	timeSinceActivity := time.Since(lastActivity)

	// Check if we're in a voice channel and it's empty
	for guildID, channelID := range c.VoiceManager.GetConnectedChannels() {
		// Check if there are tracks in the queue or active playback
		queueLength := c.QueueManager.GetQueueLength(guildID)
		playerState := c.VoiceManager.GetPlayerState(guildID)

		// Don't enter idle mode if there's active playback or queued tracks
		if queueLength > 0 || playerState == audio.StatePlaying || playerState == audio.StatePaused {
			logger.InfoLogger.Println("Active playback or queued tracks exist, not entering idle mode")
			return
		}

		if c.checkChannelEmpty(guildID, channelID) {
			logger.InfoLogger.Println("Voice channel is empty, checking if we should enter idle mode")

			if guildID == defaultGuildID && channelID == defaultVCID {
				logger.InfoLogger.Println("Already in the idle channel, enabling idle mode")
				c.Mu.Lock()
				c.IdleModeDisabled = false
				c.Mu.Unlock()
				c.startIdleMode()
			} else if !idleModeDisabled {
				logger.InfoLogger.Println("In non-idle channel that's empty, moving to idle channel")

				c.Mu.Lock()
				c.IdleModeDisabled = false
				c.Mu.Unlock()

				c.LeaveVoiceChannel(guildID)

				go func() {
					time.Sleep(2 * time.Second)
					c.startIdleMode()
				}()
			}
			return
		}
	}

	// Check if we've been idle for too long
	if timeSinceActivity.Seconds() > float64(idleTimeout) && !idleModeDisabled {
		// Check if there is any active playback or queued tracks in any guild
		for guildID := range c.VoiceManager.GetConnectedChannels() {
			queueLength := c.QueueManager.GetQueueLength(guildID)
			playerState := c.VoiceManager.GetPlayerState(guildID)

			if queueLength > 0 || playerState == audio.StatePlaying || playerState == audio.StatePaused {
				logger.InfoLogger.Println("Active playback or queued tracks, not entering idle mode despite timeout")
				return
			}
		}

		logger.InfoLogger.Printf("Bot idle for %v seconds, entering radio mode", int(timeSinceActivity.Seconds()))
		c.startIdleMode()
		return
	}
}

func (c *Client) EnableIdleMode() {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	c.IdleModeDisabled = false
	logger.InfoLogger.Println("Idle mode enabled")
}

func (c *Client) DisableIdleMode() {
	disablingIdleModeMu.Lock()
	if disablingIdleMode {
		disablingIdleModeMu.Unlock()
		return
	}
	disablingIdleMode = true
	disablingIdleModeMu.Unlock()

	// Make sure we reset the flag when we're done
	defer func() {
		disablingIdleModeMu.Lock()
		disablingIdleMode = false
		disablingIdleModeMu.Unlock()
	}()

	c.Mu.Lock()

	// Mark idle mode as disabled
	c.IdleModeDisabled = true

	// Exit idle mode if we're in it
	wasInIdleMode := c.IsInIdleMode
	c.IsInIdleMode = false

	// Update last activity time to prevent timeout-based idle mode
	c.LastActivityTime = time.Now().Add(time.Hour * 24) // Set to far future

	c.Mu.Unlock()

	// Only stop radio if we were actually in idle mode
	if wasInIdleMode && c.RadioManager.IsPlaying() {
		logger.InfoLogger.Println("Stopping radio stream due to idle mode being disabled")
		c.RadioManager.Stop()
	}

	logger.InfoLogger.Println("Idle mode disabled and prevented for 24 hours")
}

func (c *Client) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	logger.InfoLogger.Printf("Logged in as: %s#%s", s.State.User.Username, s.State.User.Discriminator)
	logger.InfoLogger.Printf("Bot is in %d servers", len(r.Guilds))

	s.UpdateGameStatus(0, "Radio Mode | Use /help")
}

func (c *Client) handleVoiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	if v.UserID == s.State.User.ID {
		// The bot was disconnected from a voice channel
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

			// Check if there are tracks in the queue - if so, try to rejoin
			if c.QueueManager.GetQueueLength(v.GuildID) > 0 {
				logger.InfoLogger.Printf("Queue has tracks, attempting to reconnect...")

				// Try to find a valid voice channel to rejoin
				lastChannel := c.VoiceManager.GetLastKnownChannel(v.GuildID)
				if lastChannel != "" {
					go func() {
						time.Sleep(1 * time.Second) // Wait a bit before reconnecting
						c.DisableIdleMode()
						err := c.RobustJoinVoiceChannel(v.GuildID, lastChannel)
						if err == nil {
							logger.InfoLogger.Printf("Successfully reconnected to continue queue playback")
							time.Sleep(500 * time.Millisecond)
							c.StartPlayback(v.GuildID)
						} else {
							logger.ErrorLogger.Printf("Failed to reconnect: %v", err)
						}
					}()
					return
				}
			} else {
				// Only enter idle mode if queue is empty
				go func() {
					time.Sleep(2 * time.Second)
					c.startIdleMode()
				}()
			}

			return
		}

		// The bot was moved to a different channel
		currentChannel := c.VoiceManager.GetConnectedChannel(v.GuildID)
		if currentChannel != "" && currentChannel != v.ChannelID {
			logger.InfoLogger.Printf("Bot was moved from channel %s to channel %s",
				currentChannel, v.ChannelID)

			c.Mu.Lock()
			isInIdleChannel := (v.ChannelID == c.DefaultVCID && v.GuildID == c.DefaultGuildID)
			wasInIdleMode := c.IsInIdleMode

			if isInIdleChannel {
				c.IsInIdleMode = true
				c.IdleModeDisabled = false
				logger.InfoLogger.Println("Bot was moved to idle channel, enabling idle mode")
			} else if wasInIdleMode {
				c.IsInIdleMode = false
				c.IdleModeDisabled = true
				logger.InfoLogger.Println("Bot was moved out of idle channel, disabling idle mode")
			}
			c.Mu.Unlock()

			c.VoiceManager.HandleChannelMove(v.GuildID, v.ChannelID)

			if !wasInIdleMode && isInIdleChannel {
				go c.RadioManager.Start()
			} else if wasInIdleMode && !isInIdleChannel {
				c.RadioManager.Stop()
			}

			return
		}
	}

	// A user joined or left a voice channel
	if v.UserID != s.State.User.ID {
		// Check if the bot is alone in a voice channel
		for guildID, channelID := range c.VoiceManager.GetConnectedChannels() {
			if c.checkChannelEmpty(guildID, channelID) {
				logger.InfoLogger.Println("Bot is alone in voice channel, checking if should move to idle channel")

				// Check if there is any active playback or queued tracks
				queueLength := c.QueueManager.GetQueueLength(guildID)
				playerState := c.VoiceManager.GetPlayerState(guildID)

				// Don't enter idle mode if there's active playback or queued tracks
				if queueLength > 0 || playerState == audio.StatePlaying || playerState == audio.StatePaused {
					logger.InfoLogger.Println("Active playback or queued tracks, not entering idle mode")
					continue
				}

				c.Mu.Lock()
				defaultGuildID := c.DefaultGuildID
				defaultVCID := c.DefaultVCID
				idleModeDisabled := c.IdleModeDisabled
				c.Mu.Unlock()

				if channelID != defaultVCID || guildID != defaultGuildID {
					c.LeaveVoiceChannel(guildID)

					c.Mu.Lock()
					c.IdleModeDisabled = false
					c.Mu.Unlock()

					go func() {
						time.Sleep(2 * time.Second)
						c.startIdleMode()
					}()
				} else if idleModeDisabled {
					c.Mu.Lock()
					c.IdleModeDisabled = false
					c.Mu.Unlock()

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
