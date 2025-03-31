package discord

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/logger"
)

type ClientConfig struct {
	Token         string
	ClientID      string
	DefaultVolume float32
	DefaultGuildID string
	DefaultVCID    string
	RadioURL      string
	IdleTimeout   int
}

type Client struct {
	token            string
	clientID         string
	session          *discordgo.Session
	
	voiceConnections map[string]*discordgo.VoiceConnection
	currentVolume    float32
	
	songQueues       map[string][]audio.Track
	playbackStatus   map[string]string
	
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
	
	stopChan         chan bool
}

func NewClient(config ClientConfig) (*Client, error) {
	if config.Token == "" {
		return nil, errors.New("discord token is required")
	}
	
	if config.ClientID == "" {
		return nil, errors.New("client ID is required")
	}
	
	client := &Client{
		token:            config.Token,
		clientID:         config.ClientID,
		currentVolume:    config.DefaultVolume,
		voiceConnections: make(map[string]*discordgo.VoiceConnection),
		songQueues:       make(map[string][]audio.Track),
		playbackStatus:   make(map[string]string),
		defaultGuildID:   config.DefaultGuildID,
		defaultVCID:      config.DefaultVCID,
		radioURL:         config.RadioURL,
		idleTimeout:      config.IdleTimeout,
		lastActivityTime: time.Now(),
		isInIdleMode:     false,
		stopChan:         make(chan bool),
	}
	
	session, err := discordgo.New("Bot " + config.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}
	
	client.session = session
	
	client.radioStreamer = NewRadioStreamer(client, config.RadioURL, config.DefaultVolume)
	
	session.AddHandler(client.handleReady)
	session.AddHandler(client.handleVoiceStateUpdate)
	
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
	
	c.startIdleChecker()
	
	return c.RefreshSlashCommands()
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
	
	return c.session.Close()
}

func (c *Client) StartActivity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.lastActivityTime = time.Now()
	
	if c.isInIdleMode {
		logger.InfoLogger.Println("Exiting idle mode due to user activity")
		c.isInIdleMode = false
		c.radioStreamer.Stop()
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
	
	c.isInIdleMode = true
	defaultGuildID := c.defaultGuildID
	defaultVCID := c.defaultVCID
	c.mu.Unlock()
	
	logger.InfoLogger.Println("Entering idle mode")
	
	err := c.JoinVoiceChannel(defaultGuildID, defaultVCID)
	if err != nil {
		logger.ErrorLogger.Printf("Failed to join default voice channel: %v", err)
		
		c.mu.Lock()
		c.isInIdleMode = false
		c.mu.Unlock()
		return
	}
	
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
	c.mu.RUnlock()
	
	if isInIdleMode {
		return
	}
	
	timeSinceActivity := time.Since(lastActivity)
	
	if timeSinceActivity.Seconds() > float64(idleTimeout) {
		logger.InfoLogger.Printf("Bot idle for %v seconds, entering radio mode", int(timeSinceActivity.Seconds()))
		c.startIdleMode()
		return
	}
	
	if vc, ok := c.GetCurrentVoiceConnection(); ok && vc != nil {
		if c.checkChannelEmpty(vc.GuildID, vc.ChannelID) {
			logger.InfoLogger.Println("Voice channel is empty, entering radio mode")
			c.startIdleMode()
			return
		}
	}
}

func (c *Client) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	logger.InfoLogger.Printf("Logged in as: %s#%s", s.State.User.Username, s.State.User.Discriminator)
	logger.InfoLogger.Printf("Bot is in %d servers", len(r.Guilds))
	
	s.UpdateGameStatus(0, "/play to add music")
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
		}
		c.mu.Unlock()
		return
	}
	
	c.mu.RLock()
	for guildID, vc := range c.voiceConnections {
		if vc != nil && vc.ChannelID == v.ChannelID && v.ChannelID != "" && v.UserID != s.State.User.ID {
			if c.checkChannelEmpty(guildID, vc.ChannelID) {
				logger.InfoLogger.Println("All users left voice channel, checking if we should enter idle mode")
				
				go func() {
					time.Sleep(3 * time.Second)
					if c.checkChannelEmpty(guildID, vc.ChannelID) {
						c.startIdleMode()
					}
				}()
			}
			break
		}
	}
	c.mu.RUnlock()
}