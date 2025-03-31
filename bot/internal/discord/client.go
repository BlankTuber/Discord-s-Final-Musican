package discord

import (
	"errors"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/logger"
)

type ClientConfig struct {
	Token         string
	ClientID      string
	DefaultVolume float32
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
	}
	
	session, err := discordgo.New("Bot " + config.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}
	
	client.session = session
	
	session.AddHandler(client.handleReady)
	session.AddHandler(client.handleVoiceStateUpdate)
	
	client.setupCommandSystem()
	
	session.Identify.Intents = discordgo.IntentsGuildVoiceStates | discordgo.IntentsGuilds
	
	return client, nil
}

func (c *Client) Connect() error {
	err := c.session.Open()
	if err != nil {
		return err
	}
	
	return c.RefreshSlashCommands()
}

func (c *Client) Disconnect() error {
	for guildID, vc := range c.voiceConnections {
		if vc != nil {
			vc.Disconnect()
		}
		delete(c.voiceConnections, guildID)
	}
	
	return c.session.Close()
}

func (c *Client) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	logger.InfoLogger.Printf("Logged in as: %s#%s", s.State.User.Username, s.State.User.Discriminator)
	logger.InfoLogger.Printf("Bot is in %d servers", len(r.Guilds))
	
	s.UpdateGameStatus(0, "/play to add music")
}

func (c *Client) handleVoiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	if v.UserID == s.State.User.ID && v.ChannelID == "" {
		if _, ok := c.voiceConnections[v.GuildID]; ok {
			delete(c.voiceConnections, v.GuildID)
			delete(c.playbackStatus, v.GuildID)
			logger.InfoLogger.Printf("Bot was disconnected from voice in guild %s", v.GuildID)
		}
	}
}

func (c *Client) JoinVoiceChannel(guildID, channelID string) error {
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
	vc, ok := c.voiceConnections[guildID]
	if !ok || vc == nil {
		return errors.New("not connected to a voice channel in this guild")
	}
	
	if err := vc.Disconnect(); err != nil {
		return fmt.Errorf("failed to disconnect from voice channel: %w", err)
	}
	
	delete(c.voiceConnections, guildID)
	delete(c.playbackStatus, guildID)
	
	return nil
}

func (c *Client) GetUserVoiceChannel(guildID, userID string) (string, error) {
	vs, err := c.session.State.VoiceState(guildID, userID)
	if err != nil || vs == nil || vs.ChannelID == "" {
		return "", errors.New("user is not in a voice channel")
	}
	
	return vs.ChannelID, nil
}