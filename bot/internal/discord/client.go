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
	DBPath         string
}

type Client struct {
	token            string
	clientID         string
	session          *discordgo.Session
	
	voiceConnections map[string]*discordgo.VoiceConnection
	currentVolume    float32
	
	songQueues       map[string][]*audio.Track
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
	
	searchResultsCache map[string][]*audio.Track
	componentHandlers  map[string]func(*discordgo.Session, *discordgo.InteractionCreate)
	
	stopChan         chan bool

	CommandsEnabled bool
	commandsMutex  sync.RWMutex
}

func (c *Client) DisableCommands() {
	c.commandsMutex.Lock()
	defer c.commandsMutex.Unlock()
	c.CommandsEnabled = false
}

func (c *Client) IsCommandsEnabled() bool {
	c.commandsMutex.RLock()
	defer c.commandsMutex.RUnlock()
	return c.CommandsEnabled
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
		songQueues:       make(map[string][]*audio.Track),
		playbackStatus:   make(map[string]string),
		players:          make(map[string]*audio.Player),
		defaultGuildID:   config.DefaultGuildID,
		defaultVCID:      config.DefaultVCID,
		radioURL:         config.RadioURL,
		idleTimeout:      config.IdleTimeout,
		lastActivityTime: time.Now(),
		isInIdleMode:     false,
		idleModeDisabled: false,
		searchResultsCache: make(map[string][]*audio.Track),
		componentHandlers:  make(map[string]func(*discordgo.Session, *discordgo.InteractionCreate)),
		stopChan:         make(chan bool),
	}
	
	session, err := discordgo.New("Bot " + config.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}
	
	client.session = session
	
	client.radioStreamer = NewRadioStreamer(client, config.RadioURL, config.DefaultVolume)
	
	client.udsClient = uds.NewClient(config.UDSPath)
	client.udsClient.SetTimeout(60 * time.Second)
	
	dbPath := filepath.Join("..", "shared", "musicbot.db")
	if config.DBPath != "" {
		dbPath = config.DBPath
	}
	
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
	session.AddHandler(client.handleComponentInteraction)
	
	audio.RegisterPlayerEventHandler(client.handlePlayerEvent)
	
	client.setupCommandSystem()
	client.initSearchComponents()
	
	session.Identify.Intents = discordgo.IntentsGuildVoiceStates | 
		discordgo.IntentsGuilds | discordgo.IntentsGuildMessages
	
	return client, nil
}

func (c *Client) Connect() error {
	err := c.session.Open()
	if err != nil {
		return err
	}
	
	err = c.udsClient.Ping()
	if err != nil {
		logger.WarnLogger.Printf("Failed to ping downloader service: %v", err)
		logger.WarnLogger.Println("Make sure the downloader service is running!")
	} else {
		logger.InfoLogger.Println("Successfully connected to downloader service")
	}
	
	c.startIdleChecker()
	c.CleanupSearchCache()
	
	go c.startIdleMode()
	
	return c.RefreshSlashCommands()
}

func (c *Client) StopAllPlayback() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	for _, player := range c.players {
		if player != nil {
			player.Stop()
		}
	}
	
	time.Sleep(100 * time.Millisecond)
}

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

func (c *Client) QueueTrackWithoutStarting(guildID string, track *audio.Track) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.songQueues[guildID]; !ok {
		c.songQueues[guildID] = make([]*audio.Track, 0)
	}

	c.songQueues[guildID] = append(c.songQueues[guildID], track)

	if c.dbManager != nil {
		go func() {
			err := c.dbManager.SaveQueue(guildID, c.songQueues[guildID])
			if err != nil {
				logger.ErrorLogger.Printf("Failed to save queue to database: %v", err)
			}
		}()
	}
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
	
	alreadyConnected := false
	if vc, ok := c.voiceConnections[defaultGuildID]; ok && vc != nil && vc.ChannelID == defaultVCID {
		alreadyConnected = true
	}
	
	c.mu.Unlock()
	
	logger.InfoLogger.Println("Entering idle mode")
	
	c.StopAllPlayback()
	
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
	
	go c.runJanitor()
	
	time.Sleep(250 * time.Millisecond)
	
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
	
	if vc, ok := c.GetCurrentVoiceConnection(); ok && vc != nil {
		if c.checkChannelEmpty(vc.GuildID, vc.ChannelID) {
			logger.InfoLogger.Println("Voice channel is empty, checking if we should enter idle mode")
			
			if vc.GuildID == defaultGuildID && vc.ChannelID == defaultVCID {
				logger.InfoLogger.Println("Already in the idle channel, enabling idle mode")
				c.mu.Lock()
				c.idleModeDisabled = false
				c.mu.Unlock()
				c.startIdleMode()
			} else if !idleModeDisabled {
				logger.InfoLogger.Println("In non-idle channel that's empty, moving to idle channel")
				
				c.mu.Lock()
				c.idleModeDisabled = false
				c.mu.Unlock()
				
				vc.Disconnect()
				
				go func() {
					time.Sleep(2 * time.Second)
					c.startIdleMode()
				}()
			}
			return
		}
	}
	
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
	
	janitorPath := "../janitor/janitor"
	
	if _, err := os.Stat(janitorPath); os.IsNotExist(err) {
		logger.WarnLogger.Printf("Janitor binary not found at %s", janitorPath)
		logger.WarnLogger.Println("To compile janitor: cd ../janitor && gcc janitor.c -o janitor -lsqlite3")
		return
	}
	
	cmd := exec.Command(janitorPath, "../shared/musicbot.db", "../shared")
	
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
		
		wasDisabled := c.idleModeDisabled
		c.idleModeDisabled = false
		c.mu.Unlock()
		
		if wasDisabled {
			logger.InfoLogger.Println("Bot was disconnected and idle mode was disabled, re-enabling idle mode")
		}
		
		go func() {
			time.Sleep(2 * time.Second)
			c.startIdleMode()
		}()
		
		return
	}
	
	if v.UserID == s.State.User.ID && v.ChannelID != "" {
		c.mu.Lock()
		storedVC, exists := c.voiceConnections[v.GuildID]
		
		if exists && storedVC != nil && storedVC.ChannelID != v.ChannelID {
			logger.InfoLogger.Printf("Bot was moved from channel %s to channel %s", 
				storedVC.ChannelID, v.ChannelID)
			
			c.voiceConnections[v.GuildID] = nil
			
			isInIdleChannel := (v.ChannelID == c.defaultVCID && v.GuildID == c.defaultGuildID)
			wasInIdleMode := c.isInIdleMode
			
			if isInIdleChannel {
				c.isInIdleMode = true
				c.idleModeDisabled = false
				logger.InfoLogger.Println("Bot was moved to idle channel, enabling idle mode")
			} else if wasInIdleMode {
				c.isInIdleMode = false
				c.idleModeDisabled = true
				logger.InfoLogger.Println("Bot was moved out of idle channel, disabling idle mode")
			}
			
			streamer := c.radioStreamer
			c.mu.Unlock()
			
			if !wasInIdleMode && isInIdleChannel {
				go streamer.Start()
			} else if wasInIdleMode && !isInIdleChannel {
				streamer.Stop()
			}
			
			return
		}
		c.mu.Unlock()
	}
	
	if v.UserID != s.State.User.ID {
		c.mu.RLock()
		var botsVC *discordgo.VoiceConnection
		var botsGuildID string
		
		for guildID, vc := range c.voiceConnections {
			if vc != nil {
				botsVC = vc
				botsGuildID = guildID
				break
			}
		}
		
		defaultGuildID := c.defaultGuildID
		defaultVCID := c.defaultVCID
		idleModeDisabled := c.idleModeDisabled
		c.mu.RUnlock()
		
		if botsVC != nil {
			go func() {
				time.Sleep(1 * time.Second)
				
				isEmpty := c.checkChannelEmpty(botsGuildID, botsVC.ChannelID)
				
				if isEmpty {
					logger.InfoLogger.Println("Bot is alone in voice channel, moving to idle channel")
					
					if botsVC.ChannelID != defaultVCID || botsGuildID != defaultGuildID {
						botsVC.Disconnect()
						
						c.mu.Lock()
						c.idleModeDisabled = false
						c.mu.Unlock()
						
						c.startIdleMode()
					} else if idleModeDisabled {
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

func (c *Client) SetVolume(guildID string, volume float32) {
	c.mu.Lock()
	c.currentVolume = volume
	c.mu.Unlock()
	
	if player, ok := c.players[guildID]; ok {
		player.SetVolume(volume)
	}
	
	c.radioStreamer.SetVolume(volume)
}