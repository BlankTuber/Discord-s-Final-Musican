package discord

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/database"
	"quidque.com/discord-musican/internal/logger"
	"quidque.com/discord-musican/internal/uds"
)

type ClientConfig struct {
	Token          string
	ClientID       string
	DefaultVolume  float32
	DefaultGuildID string
	DefaultVCID    string
	RadioURL       string
	IdleTimeout    int
	UDSPath        string
}

type Client struct {
	token            string
	clientID         string
	session          *discordgo.Session
	
	voiceConnections map[string]*discordgo.VoiceConnection
	currentVolume    float32
	
	songQueues       map[string][]audio.Track
	playbackStatus   map[string]string
	players          map[string]*audio.Player
	
	commands         *CommandRegistry
	
	defaultGuildID   string
	defaultVCID      string
	radioURL         string
	idleTimeout      int
	radioStreamer    *RadioStreamer
	
	mu               sync.RWMutex
	
	lastActivityTime time.Time
	idleCheckTicker  *time.Ticker
	isInIdleMode     bool
	idleModeDisabled bool
	
	udsClient        *uds.Client
	dbManager        *database.Manager
	
	stopChan         chan bool
}

func NewClient(config ClientConfig) (*Client, error) {
	if config.Token == "" {
		return nil, errors.New("discord token is required")
	}
	
	if config.ClientID == "" {
		return nil, errors.New("client ID is required")
	}
	
	if config.UDSPath == "" {
		config.UDSPath = "/tmp/downloader.sock"
	}
	
	client := &Client{
		token:            config.Token,
		clientID:         config.ClientID,
		currentVolume:    config.DefaultVolume,
		voiceConnections: make(map[string]*discordgo.VoiceConnection),
		songQueues:       make(map[string][]audio.Track),
		playbackStatus:   make(map[string]string),
		players:          make(map[string]*audio.Player),
		defaultGuildID:   config.DefaultGuildID,
		defaultVCID:      config.DefaultVCID,
		radioURL:         config.RadioURL,
		idleTimeout:      config.IdleTimeout,
		lastActivityTime: time.Now(),
		isInIdleMode:     false,
		idleModeDisabled: false,
		stopChan:         make(chan bool),
	}
	
	session, err := discordgo.New("Bot " + config.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}
	
	client.session = session
	
	client.radioStreamer = NewRadioStreamer(client, config.RadioURL, config.DefaultVolume)
	
	client.udsClient = uds.NewClient(config.UDSPath)
	client.udsClient.SetTimeout(30 * time.Second)
	
	// Initialize database manager
	dbPath := filepath.Join("..", "shared", "musicbot.db")
	dbManager, err := database.NewManager(dbPath)
	if err != nil {
		logger.WarnLogger.Printf("Failed to connect to database: %v", err)
		logger.WarnLogger.Println("Database features will be disabled")
	} else {
		logger.InfoLogger.Println("Successfully connected to database")
		client.dbManager = dbManager
	}
	
	session.AddHandler(client.handleReady)
	session.AddHandler(client.handleVoiceStateUpdate)
	
	audio.RegisterPlayerEventHandler(client.handlePlayerEvent)
	
	client.setupCommandSystem()
	
	session.Identify.Intents = discordgo.IntentsGuildVoiceStates | 
		discordgo.IntentsGuilds | discordgo.IntentsGuildMessages
	
	return client, nil
}

func (c *Client) Connect() error {
	err := c.session.Open()
	if err != nil {
		return err
	}
	
	// Test UDS connection
	err = c.udsClient.Ping()
	if err != nil {
		logger.WarnLogger.Printf("Failed to ping downloader service: %v", err)
		logger.WarnLogger.Println("Make sure the downloader service is running!")
	} else {
		logger.InfoLogger.Println("Successfully connected to downloader service")
	}
	
	c.startIdleChecker()
	
	go c.startIdleMode()
	
	return c.RefreshSlashCommands()
}

// This method has been removed as it duplicates the one in handler.go

func (c *Client) Disconnect() error {
	if c.idleCheckTicker != nil {
		c.idleCheckTicker.Stop()
	}
	
	if c.radioStreamer != nil {
		c.radioStreamer.Stop()
	}
	
	close(c.stopChan)
	
	for guildID, vc := range c.voiceConnections {
		if vc != nil {
			vc.Disconnect()
		}
		delete(c.voiceConnections, guildID)
	}
	
	// Close database connection
	if c.dbManager != nil {
		if err := c.dbManager.Close(); err != nil {
			logger.ErrorLogger.Printf("Error closing database: %v", err)
		}
	}
	
	return c.session.Close()
}

func (c *Client) StartActivity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.lastActivityTime = time.Now()
}

func (c *Client) GetCurrentVoiceConnection() (*discordgo.VoiceConnection, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	for _, vc := range c.voiceConnections {
		if vc != nil {
			return vc, true
		}
	}
	
	return nil, false
}

func (c *Client) JoinVoiceChannel(guildID, channelID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.lastActivityTime = time.Now()
	
	if vc, ok := c.voiceConnections[guildID]; ok {
		if vc.ChannelID == channelID {
			return nil
		}
		
		vc.Disconnect()
	}
	
	vc, err := c.session.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return fmt.Errorf("failed to join voice channel: %w", err)
	}
	
	c.voiceConnections[guildID] = vc
	c.playbackStatus[guildID] = "idle"
	
	return nil
}

func (c *Client) LeaveVoiceChannel(guildID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	vc, ok := c.voiceConnections[guildID]
	if !ok || vc == nil {
		return errors.New("not connected to a voice channel in this guild")
	}
	
	if c.isInIdleMode {
		c.radioStreamer.Stop()
		c.isInIdleMode = false
	}
	
	if player, exists := c.players[guildID]; exists {
		player.Stop()
		delete(c.players, guildID)
	}
	
	if err := vc.Disconnect(); err != nil {
		return fmt.Errorf("failed to disconnect from voice channel: %w", err)
	}
	
	delete(c.voiceConnections, guildID)
	delete(c.playbackStatus, guildID)
	
	return nil
}

func (c *Client) SetDefaultVoiceChannel(guildID, channelID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.defaultGuildID = guildID
	c.defaultVCID = channelID
	
	logger.InfoLogger.Printf("Default voice channel set to %s in guild %s", channelID, guildID)
}

func (c *Client) SetRadioURL(url string) {
	c.mu.Lock()
	c.radioURL = url
	c.mu.Unlock()
	
	c.radioStreamer.SetStream(url)
	logger.InfoLogger.Printf("Radio URL set to %s", url)
}

func (c *Client) GetUserVoiceChannel(guildID, userID string) (string, error) {
	vs, err := c.session.State.VoiceState(guildID, userID)
	if err != nil || vs == nil || vs.ChannelID == "" {
		return "", errors.New("user is not in a voice channel")
	}
	
	return vs.ChannelID, nil
}

func (c *Client) checkChannelEmpty(guildID, channelID string) bool {
	guild, err := c.session.State.Guild(guildID)
	if err != nil {
		logger.ErrorLogger.Printf("Error getting guild %s: %v", guildID, err)
		return false
	}
	
	botID := c.session.State.User.ID
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
	c.mu.Lock()
	
	if c.isInIdleMode {
		c.mu.Unlock()
		return
	}
	
	if c.defaultGuildID == "" || c.defaultVCID == "" {
		logger.WarnLogger.Println("Cannot enter idle mode: default voice channel not configured")
		c.mu.Unlock()
		return
	}
	
	if c.idleModeDisabled {
		logger.InfoLogger.Println("Idle mode is currently disabled")
		c.mu.Unlock()
		return
	}
	
	c.isInIdleMode = true
	defaultGuildID := c.defaultGuildID
	defaultVCID := c.defaultVCID
	
	// Check if already connected to the default channel
	alreadyConnected := false
	if vc, ok := c.voiceConnections[defaultGuildID]; ok && vc != nil && vc.ChannelID == defaultVCID {
		alreadyConnected = true
	}
	
	c.mu.Unlock()
	
	logger.InfoLogger.Println("Entering idle mode")
	
	// Only join if not already connected to the right channel
	if !alreadyConnected {
		err := c.JoinVoiceChannel(defaultGuildID, defaultVCID)
		if err != nil {
			logger.ErrorLogger.Printf("Failed to join default voice channel: %v", err)
			
			c.mu.Lock()
			c.isInIdleMode = false
			c.mu.Unlock()
			return
		}
	} else {
		logger.InfoLogger.Println("Already in the default voice channel, staying connected")
	}
	
	// Invoke the janitor as a separate process
	go c.runJanitor()
	
	c.radioStreamer.Start()
	
	c.session.UpdateGameStatus(0, "Radio Mode | Use /help")
}

func (c *Client) startIdleChecker() {
	c.idleCheckTicker = time.NewTicker(30 * time.Second)
	
	go func() {
		for {
			select {
			case <-c.stopChan:
				return
			case <-c.idleCheckTicker.C:
				c.checkIdleState()
			}
		}
	}()
}

func (c *Client) checkIdleState() {
	c.mu.RLock()
	lastActivity := c.lastActivityTime
	isInIdleMode := c.isInIdleMode
	idleTimeout := c.idleTimeout
	idleModeDisabled := c.idleModeDisabled
	defaultGuildID := c.defaultGuildID
	defaultVCID := c.defaultVCID
	c.mu.RUnlock()
	
	if isInIdleMode {
		return
	}
	
	timeSinceActivity := time.Since(lastActivity)
	
	// Check if we're in any voice channel
	if vc, ok := c.GetCurrentVoiceConnection(); ok && vc != nil {
		// Always check if current channel is empty, regardless of idle timeout
		if c.checkChannelEmpty(vc.GuildID, vc.ChannelID) {
			logger.InfoLogger.Println("Voice channel is empty, checking if we should enter idle mode")
			
			// If we're already in the idle channel, just enable idle mode
			if vc.GuildID == defaultGuildID && vc.ChannelID == defaultVCID {
				logger.InfoLogger.Println("Already in the idle channel, enabling idle mode")
				c.mu.Lock()
				c.idleModeDisabled = false
				c.mu.Unlock()
				c.startIdleMode()
			} else if !idleModeDisabled {
				// If we're in a different channel and idle mode isn't disabled,
				// disconnect and move to idle channel
				logger.InfoLogger.Println("In non-idle channel that's empty, moving to idle channel")
				
				// Reset idle mode disabled flag
				c.mu.Lock()
				c.idleModeDisabled = false
				c.mu.Unlock()
				
				// Disconnect from current channel
				vc.Disconnect()
				
				// Schedule idle mode reconnect with a slight delay
				go func() {
					time.Sleep(2 * time.Second)
					c.startIdleMode()
				}()
			}
			return
		}
	}
	
	// Regular timeout check
	if timeSinceActivity.Seconds() > float64(idleTimeout) && !idleModeDisabled {
		logger.InfoLogger.Printf("Bot idle for %v seconds, entering radio mode", int(timeSinceActivity.Seconds()))
		c.startIdleMode()
		return
	}
}

func (c *Client) EnableIdleMode() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.idleModeDisabled = false
	logger.InfoLogger.Println("Idle mode enabled")
}

func (c *Client) DisableIdleMode() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.idleModeDisabled = true
	logger.InfoLogger.Println("Idle mode disabled")
}

func (c *Client) runJanitor() {
	logger.InfoLogger.Println("Running janitor to clean up old files")
	
	// Path to the janitor binary
	janitorPath := "../janitor/janitor"
	
	// Check if janitor binary exists
	if _, err := os.Stat(janitorPath); os.IsNotExist(err) {
		logger.WarnLogger.Printf("Janitor binary not found at %s", janitorPath)
		logger.WarnLogger.Println("To compile janitor: cd ../janitor && gcc janitor.c -o janitor -lsqlite3")
		return
	}
	
	// Execute the janitor binary with appropriate paths
	cmd := exec.Command(janitorPath, "../shared/musicbot.db", "../shared")
	
	// Capture output for logging
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.ErrorLogger.Printf("Failed to run janitor: %v", err)
		logger.ErrorLogger.Printf("Janitor output: %s", string(output))
		return
	}
	
	logger.InfoLogger.Printf("Janitor completed successfully: %s", string(output))
}

func (c *Client) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	logger.InfoLogger.Printf("Logged in as: %s#%s", s.State.User.Username, s.State.User.Discriminator)
	logger.InfoLogger.Printf("Bot is in %d servers", len(r.Guilds))
	
	s.UpdateGameStatus(0, "Radio Mode | Use /help")
}

func (c *Client) handleVoiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	// Bot was disconnected from voice
	if v.UserID == s.State.User.ID && v.ChannelID == "" {
		c.mu.Lock()
		if _, ok := c.voiceConnections[v.GuildID]; ok {
			delete(c.voiceConnections, v.GuildID)
			delete(c.playbackStatus, v.GuildID)
			logger.InfoLogger.Printf("Bot was disconnected from voice in guild %s", v.GuildID)
			
			if c.isInIdleMode {
				c.isInIdleMode = false
				c.radioStreamer.Stop()
			}
			
			if player, exists := c.players[v.GuildID]; exists {
				player.Stop()
				delete(c.players, v.GuildID)
			}
		}
		
		// Reset idle mode disabled when bot is disconnected
		wasDisabled := c.idleModeDisabled
		c.idleModeDisabled = false
		c.mu.Unlock()
		
		// If bot was disconnected externally, immediately go back to idle mode
		if wasDisabled {
			logger.InfoLogger.Println("Bot was disconnected and idle mode was disabled, re-enabling idle mode")
		}
		
		// Schedule idle mode reconnect with a slight delay to ensure clean disconnect
		go func() {
			time.Sleep(2 * time.Second)
			c.startIdleMode()
		}()
		
		return
	}
	
	// Bot was moved to a different voice channel
	if v.UserID == s.State.User.ID && v.ChannelID != "" {
		c.mu.Lock()
		storedVC, exists := c.voiceConnections[v.GuildID]
		
		// Update our internal tracking of the voice connection
		if exists && storedVC != nil && storedVC.ChannelID != v.ChannelID {
			logger.InfoLogger.Printf("Bot was moved from channel %s to channel %s", 
				storedVC.ChannelID, v.ChannelID)
			
			// Update our internal tracking
			c.voiceConnections[v.GuildID] = nil // Will be refreshed on next check
			
			// Check if moved to idle channel
			isInIdleChannel := (v.ChannelID == c.defaultVCID && v.GuildID == c.defaultGuildID)
			wasInIdleMode := c.isInIdleMode
			
			if isInIdleChannel {
				// If moved to idle channel, enable idle mode
				c.isInIdleMode = true
				c.idleModeDisabled = false
				logger.InfoLogger.Println("Bot was moved to idle channel, enabling idle mode")
			} else if wasInIdleMode {
				// If was in idle mode but moved elsewhere, disable idle mode
				c.isInIdleMode = false
				c.idleModeDisabled = true
				logger.InfoLogger.Println("Bot was moved out of idle channel, disabling idle mode")
			}
			
			// Restart radio streamer to adapt to the new channel
			streamer := c.radioStreamer
			c.mu.Unlock()
			
			if !wasInIdleMode && isInIdleChannel {
				// If entering idle mode, start radio
				go streamer.Start()
			} else if wasInIdleMode && !isInIdleChannel {
				// If leaving idle mode, stop radio
				streamer.Stop()
			}
			
			return
		}
		c.mu.Unlock()
	}
	
	// Any user (not the bot) left or joined a voice channel
	if v.UserID != s.State.User.ID {
		// Check all voice connections where the bot is present
		c.mu.RLock()
		var botsVC *discordgo.VoiceConnection
		var botsGuildID string
		
		// Find any voice channel where the bot is present
		for guildID, vc := range c.voiceConnections {
			if vc != nil {
				// Store reference to bot's current VC
				botsVC = vc
				botsGuildID = guildID
				break
			}
		}
		
		// Get default idle channel info
		defaultGuildID := c.defaultGuildID
		defaultVCID := c.defaultVCID
		idleModeDisabled := c.idleModeDisabled
		c.mu.RUnlock()
		
		// If bot is in a voice channel
		if botsVC != nil {
			// Check if the channel is now empty (with delay for state updates)
			go func() {
				time.Sleep(1 * time.Second)
				
				// Check if bot is alone in its current channel
				isEmpty := c.checkChannelEmpty(botsGuildID, botsVC.ChannelID)
				
				if isEmpty {
					logger.InfoLogger.Println("Bot is alone in voice channel, moving to idle channel")
					
					// If bot is alone in ANY channel that's not the idle channel, move to the idle channel
					if botsVC.ChannelID != defaultVCID || botsGuildID != defaultGuildID {
						// Leave current channel
						botsVC.Disconnect()
						
						// Reset disabled flag and enter idle mode
						c.mu.Lock()
						c.idleModeDisabled = false
						c.mu.Unlock()
						
						// Start idle mode (which will connect to the default channel)
						c.startIdleMode()
					} else if idleModeDisabled {
						// If already in the idle channel but idle mode is disabled, enable it
						c.mu.Lock()
						c.idleModeDisabled = false
						c.mu.Unlock()
						
						c.startIdleMode()
					}
				}
			}()
		}
	}
}

// Music playback functions
func (c *Client) ProcessSong(guildID, url, requesterName string, callback func(string)) {
	c.mu.RLock()
	vc, ok := c.voiceConnections[guildID]
	c.mu.RUnlock()
	
	if !ok || vc == nil {
		callback("❌ Bot is not connected to a voice channel.")
		return
	}
	
	// First, check if this song is already in the database
	var track *audio.Track
	if c.dbManager != nil {
		var err error
		track, err = c.dbManager.GetTrackByURL(url)
		if err != nil {
			logger.ErrorLogger.Printf("Error querying track from database: %v", err)
		}
	}
	
	// If track not found in database, download it
	if track == nil {
		results, err := c.udsClient.DownloadAudio(url, DefaultMaxDuration, DefaultMaxSize, false)
		if err != nil {
			callback(fmt.Sprintf("❌ Failed to download song: %s", err.Error()))
			return
		}
		
		if statusMsg, ok := results["status"].(string); ok && statusMsg == "error" {
			errorMsg := "Unknown error"
			if msg, ok := results["message"].(string); ok {
				errorMsg = msg
			}
			callback(fmt.Sprintf("❌ Download failed: %s", errorMsg))
			return
		}
		
		// Try to get track info from database again after download
		if c.dbManager != nil {
			var err error
			track, err = c.dbManager.GetTrackByURL(url)
			if err != nil {
				logger.ErrorLogger.Printf("Error querying track from database after download: %v", err)
			}
		}
		
		// If still not found, create a minimal track
		if track == nil {
			track = &audio.Track{
				Title:         "Unknown Song",
				URL:           url,
				Requester:     requesterName,
				RequestedAt:   time.Now().Unix(),
				DownloadStatus: "completed",
			}
		}
	}
	
	// Always update requester and requested time
	track.Requester = requesterName
	track.RequestedAt = time.Now().Unix()
	
	c.addTrackToQueue(guildID, *track)
	
	callback(fmt.Sprintf("✅ Added to queue: **%s**", track.Title))
}

func (c *Client) ProcessPlaylist(guildID, url, requesterName string, callback func(string)) {
	c.mu.RLock()
	vc, ok := c.voiceConnections[guildID]
	c.mu.RUnlock()
	
	if !ok || vc == nil {
		callback("❌ Bot is not connected to a voice channel.")
		return
	}
	
	results, err := c.udsClient.DownloadPlaylist(url, DefaultPlaylistMax, DefaultMaxDuration, DefaultMaxSize, false)
	if err != nil {
		callback(fmt.Sprintf("❌ Failed to download playlist: %s", err.Error()))
		return
	}
	
	if statusMsg, ok := results["status"].(string); ok && statusMsg == "error" {
		errorMsg := "Unknown error"
		if msg, ok := results["message"].(string); ok {
			errorMsg = msg
		}
		callback(fmt.Sprintf("❌ Download failed: %s", errorMsg))
		return
	}
	
	count := 0
	if countVal, ok := results["count"].(float64); ok {
		count = int(countVal)
	}
	
	if count == 0 {
		callback("❌ No songs could be downloaded from this playlist.")
		return
	}
	
	// Add tracks to queue
	for i := 0; i < count; i++ {
		track := &audio.Track{
			Title:         fmt.Sprintf("Playlist Song %d", i+1),
			URL:           url,
			Requester:     requesterName,
			RequestedAt:   time.Now().Unix(),
			DownloadStatus: "completed",
		}
		
		c.addTrackToQueue(guildID, *track)
	}
	
	callback(fmt.Sprintf("✅ Added %d songs from playlist to the queue!", count))
}

func (c *Client) ProcessSearch(guildID, query, requesterName string, callback func(string)) {
	results, err := c.udsClient.Search(query, DefaultSearchPlatform, DefaultSearchCount, false)
	if err != nil {
		callback(fmt.Sprintf("❌ Search failed: %s", err.Error()))
		return
	}
	
	if len(results) == 0 {
		callback(fmt.Sprintf("❌ No results found for \"%s\"", query))
		return
	}
	
	// Use the first result
	result := results[0]
	
	title, _ := result["title"].(string)
	url, _ := result["url"].(string)
	durationFloat, _ := result["duration"].(float64)
	duration := int(durationFloat)
	thumbnail, _ := result["thumbnail"].(string)
	uploader, _ := result["uploader"].(string)
	
	// Download the track
	go func() {
		callback(fmt.Sprintf("🔍 Found: **%s**\n⏳ Downloading...", title))
		
		downloadResults, err := c.udsClient.DownloadAudio(url, DefaultMaxDuration, DefaultMaxSize, false)
		if err != nil {
			callback(fmt.Sprintf("❌ Failed to download song: %s", err.Error()))
			return
		}
		
		if statusMsg, ok := downloadResults["status"].(string); ok && statusMsg == "error" {
			errorMsg := "Unknown error"
			if msg, ok := downloadResults["message"].(string); ok {
				errorMsg = msg
			}
			callback(fmt.Sprintf("❌ Download failed: %s", errorMsg))
			return
		}
		 
		// Create track
		track := &audio.Track{
			Title:         title,
			URL:           url,
			Duration:      duration,
			Requester:     requesterName,
			RequestedAt:   time.Now().Unix(),
			ArtistName:    uploader,
			ThumbnailURL:  thumbnail,
			DownloadStatus: "completed",
		}
		
		c.addTrackToQueue(guildID, *track)
		
		callback(fmt.Sprintf("✅ Added to queue: **%s**", title))
	}()
}

func (c *Client) addTrackToQueue(guildID string, track audio.Track) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if _, ok := c.songQueues[guildID]; !ok {
		c.songQueues[guildID] = make([]audio.Track, 0)
	}
	
	c.songQueues[guildID] = append(c.songQueues[guildID], track)
	
	// If this is the only song in the queue, start playing
	if len(c.songQueues[guildID]) == 1 {
		c.startPlayer(guildID)
	}
}

func (c *Client) startPlayer(guildID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if len(c.songQueues[guildID]) == 0 {
		return
	}
	
	// Make sure we have a voice connection
	vc, ok := c.voiceConnections[guildID]
	if !ok || vc == nil {
		logger.ErrorLogger.Printf("Cannot play in guild %s: not connected to voice", guildID)
		return
	}
	
	// Stop the radio if it's playing
	if c.isInIdleMode {
		c.radioStreamer.Stop()
		c.isInIdleMode = false
	}
	
	// Create a player if we don't have one
	if _, ok := c.players[guildID]; !ok {
		c.players[guildID] = audio.NewPlayer(vc)
	}
	
	player := c.players[guildID]
	player.SetVolume(c.currentVolume)
	
	nextTrack := c.songQueues[guildID][0]
	c.songQueues[guildID] = c.songQueues[guildID][1:]
	
	go func() {
		player.QueueTrack(&nextTrack)
	}()
}

func (c *Client) handlePlayerEvent(event string, data interface{}) {
	switch event {
	case "track_start":
		if track, ok := data.(*audio.Track); ok {
			logger.InfoLogger.Printf("Started playing track: %s", track.Title)
			c.session.UpdateGameStatus(0, fmt.Sprintf("🎵 %s", track.Title))
			
			// Update database play count
			if c.dbManager != nil && track.URL != "" {
				go func() {
					if err := c.dbManager.IncrementPlayCount(track.URL); err != nil {
						logger.ErrorLogger.Printf("Failed to update play count: %v", err)
					}
				}()
			}
		}
	case "track_end":
		if track, ok := data.(*audio.Track); ok {
			logger.InfoLogger.Printf("Finished playing track: %s", track.Title)
		}
	case "queue_end":
		logger.InfoLogger.Println("Queue ended")
		c.session.UpdateGameStatus(0, "Queue is empty | Use /play")
		
		// Wait a bit and then check if we should return to idle mode
		go func() {
			time.Sleep(5 * time.Second)
			c.checkIdleState()
		}()
	}
}

func (c *Client) GetQueueInfo(guildID string) ([]audio.Track, *audio.Track) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	var queue []audio.Track
	if q, ok := c.songQueues[guildID]; ok {
		queue = make([]audio.Track, len(q))
		copy(queue, q)
	} else {
		queue = make([]audio.Track, 0)
	}
	
	var currentTrack *audio.Track
	if player, ok := c.players[guildID]; ok {
		currentTrack = player.GetCurrentTrack()
	}
	
	return queue, currentTrack
}

func (c *Client) GetCurrentTrack(guildID string) *audio.Track {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if player, ok := c.players[guildID]; ok {
		return player.GetCurrentTrack()
	}
	
	return nil
}

func (c *Client) SkipSong(guildID string) bool {
	c.mu.RLock()
	player, ok := c.players[guildID]
	c.mu.RUnlock()
	
	if !ok || player.GetState() != audio.StatePlaying {
		return false
	}
	
	player.Skip()
	return true
}

func (c *Client) ClearQueue(guildID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.songQueues[guildID] = make([]audio.Track, 0)
	
	if player, ok := c.players[guildID]; ok {
		player.ClearQueue()
	}
}

func (c *Client) SetVolume(guildID string, volume float32) {
	c.mu.Lock()
	c.currentVolume = volume
	c.mu.Unlock()
	
	// Set volume for player
	if player, ok := c.players[guildID]; ok {
		player.SetVolume(volume)
	}
	
	// Also set volume for radio
	c.radioStreamer.SetVolume(volume)
}