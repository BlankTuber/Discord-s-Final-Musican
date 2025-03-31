package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/logger"
)

// Register radio-related commands
func registerRadioCommands(registry *CommandRegistry) {
	registry.Register(&SetDefaultVCCommand{})
	registry.Register(&RadioURLCommand{})
	registry.Register(&RadioVolumeCommand{})
	registry.Register(&RadioStartCommand{})
	registry.Register(&RadioStopCommand{})
}

// SetDefaultVCCommand sets the default voice channel for idle mode
type SetDefaultVCCommand struct{}

func (c *SetDefaultVCCommand) Name() string {
	return "setidlevc"
}

func (c *SetDefaultVCCommand) Description() string {
	return "Set the default voice channel for idle radio mode"
}

func (c *SetDefaultVCCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionChannel,
			Name:        "channel",
			Description: "The voice channel to use when idle",
			Required:    true,
		},
	}
}

func (c *SetDefaultVCCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	// Acknowledge the command
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	// Get the channel option
	options := i.ApplicationCommandData().Options
	channelOption := options[0].ChannelValue(s)
	
	// Validate that it's a voice channel
	channel, err := s.Channel(channelOption.ID)
	if err != nil || channel.Type != discordgo.ChannelTypeGuildVoice {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("The selected channel is not a voice channel. Please choose a voice channel."),
		})
		return
	}
	
	// Set the default voice channel
	client.SetDefaultVoiceChannel(i.GuildID, channel.ID)
	
	// Respond with success
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("✅ Default idle voice channel set to **" + channel.Name + "**"),
	})
	
	logger.InfoLogger.Printf("Default idle voice channel set to %s (%s) in guild %s", 
		channel.Name, channel.ID, i.GuildID)
}

// RadioURLCommand sets the radio stream URL
type RadioURLCommand struct{}

func (c *RadioURLCommand) Name() string {
	return "radiourl"
}

func (c *RadioURLCommand) Description() string {
	return "Set the URL for the radio stream"
}

func (c *RadioURLCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "url",
			Description: "The URL of the radio stream",
			Required:    true,
		},
	}
}

func (c *RadioURLCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	// Acknowledge the command
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	// Get the URL option
	options := i.ApplicationCommandData().Options
	url := options[0].StringValue()
	
	// Set the radio URL
	client.SetRadioURL(url)
	
	// Respond with success
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("✅ Radio stream URL set to " + url),
	})
}

// RadioVolumeCommand sets the radio volume
type RadioVolumeCommand struct{}

func (c *RadioVolumeCommand) Name() string {
	return "radiovolume"
}

func (c *RadioVolumeCommand) Description() string {
	return "Set the volume for the radio stream (0.0 to 1.0)"
}

func (c *RadioVolumeCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionNumber,
			Name:        "volume",
			Description: "The volume level (0.0 to 1.0)",
			Required:    true,
			MinValue:    floatPtr(0.0),
			MaxValue:    1.0,
		},
	}
}

func (c *RadioVolumeCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	// Acknowledge the command
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	// Get the volume option
	options := i.ApplicationCommandData().Options
	volume := float32(options[0].FloatValue())
	
	// Set the volume
	client.mu.Lock()
	client.currentVolume = volume
	streamer := client.radioStreamer
	client.mu.Unlock()
	
	streamer.SetVolume(volume)
	
	// Respond with success
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Radio volume set to %.1f", volume)),
	})
}

// RadioStartCommand manually starts the radio
type RadioStartCommand struct{}

func (c *RadioStartCommand) Name() string {
	return "radiostart"
}

func (c *RadioStartCommand) Description() string {
	return "Manually start the radio stream in the current voice channel"
}

func (c *RadioStartCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *RadioStartCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	// Acknowledge the command
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	// Check if the user is in a voice channel
	channelID, err := client.GetUserVoiceChannel(i.GuildID, i.Member.User.ID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel to use this command."),
		})
		return
	}
	
	// Join the voice channel
	err = client.JoinVoiceChannel(i.GuildID, channelID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to join voice channel: " + err.Error()),
		})
		return
	}
	
	// Set this guild as the default for idle mode
	client.SetDefaultVoiceChannel(i.GuildID, channelID)
	
	// Start radio mode
	client.mu.Lock()
	client.isInIdleMode = true
	streamer := client.radioStreamer
	client.mu.Unlock()
	
	// Start the stream
	go streamer.Start()
	
	// Update presence
	s.UpdateGameStatus(0, "Radio Mode | Use /help")
	
	// Respond with success
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("✅ Radio mode started!"),
	})
}

// RadioStopCommand stops the radio
type RadioStopCommand struct{}

func (c *RadioStopCommand) Name() string {
	return "radiostop"
}

func (c *RadioStopCommand) Description() string {
	return "Stop the radio stream"
}

func (c *RadioStopCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *RadioStopCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	// Acknowledge the command
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	client.mu.Lock()
	isInIdleMode := client.isInIdleMode
	streamer := client.radioStreamer
	client.isInIdleMode = false
	client.mu.Unlock()
	
	if !isInIdleMode {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Radio is not currently playing."),
		})
		return
	}
	
	// Stop the radio
	streamer.Stop()
	
	// Update presence
	s.UpdateGameStatus(0, "/play to add music")
	
	// Respond with success
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("✅ Radio stream stopped."),
	})
}

// Helper for float pointers
func floatPtr(f float64) *float64 {
	return &f
}