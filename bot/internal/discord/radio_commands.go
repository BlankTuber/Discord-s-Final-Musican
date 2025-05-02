package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/logger"
)

func registerRadioCommands(registry *CommandRegistry) {
	registry.Register(&SetDefaultVCCommand{})
	registry.Register(&RadioURLCommand{})
	registry.Register(&RadioVolumeCommand{})
	registry.Register(&RadioStartCommand{})
	registry.Register(&RadioStopCommand{})
}

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
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	options := i.ApplicationCommandData().Options
	channelOption := options[0].ChannelValue(s)
	
	channel, err := s.Channel(channelOption.ID)
	if err != nil || channel.Type != discordgo.ChannelTypeGuildVoice {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("The selected channel is not a voice channel. Please choose a voice channel."),
		})
		return
	}
	
	client.SetDefaultVoiceChannel(i.GuildID, channel.ID)
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("✅ Default idle voice channel set to **" + channel.Name + "**"),
	})
	
	logger.InfoLogger.Printf("Default idle voice channel set to %s (%s) in guild %s", 
		channel.Name, channel.ID, i.GuildID)
}

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
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	options := i.ApplicationCommandData().Options
	url := options[0].StringValue()
	
	client.SetRadioURL(url)
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("✅ Radio stream URL set to " + url),
	})
}

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
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	options := i.ApplicationCommandData().Options
	volume := float32(options[0].FloatValue())
	
	client.mu.Lock()
	client.currentVolume = volume
	streamer := client.radioStreamer
	client.mu.Unlock()
	
	streamer.SetVolume(volume)
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Radio volume set to %.2f", volume)),
	})
}

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
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	channelID, err := client.GetUserVoiceChannel(i.GuildID, i.Member.User.ID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel to use this command."),
		})
		return
	}
	
	// Check if user is in the default idle VC
	client.mu.RLock()
	isInIdleVC := (channelID == client.defaultVCID && i.GuildID == client.defaultGuildID)
	client.mu.RUnlock()
	
	// Only set default VC if we're in the idle VC
	if isInIdleVC {
		// Enable idle mode when radio is started in the default channel
		client.EnableIdleMode()
	} else {
		// When starting radio in a non-default channel, disable idle mode
		client.DisableIdleMode()
	}
	
	// Check if already connected to the right channel
	currentVC := false
	client.mu.RLock()
	if vc, ok := client.voiceConnections[i.GuildID]; ok && vc != nil && vc.ChannelID == channelID {
		currentVC = true
	}
	client.mu.RUnlock()
	
	// Only join if not already in the right channel
	if !currentVC {
		err = client.JoinVoiceChannel(i.GuildID, channelID)
		if err != nil {
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ Failed to join voice channel: " + err.Error()),
			})
			return
		}
	}
	
	client.mu.Lock()
	client.isInIdleMode = isInIdleVC
	streamer := client.radioStreamer
	client.mu.Unlock()
	
	go streamer.Start()
	
	s.UpdateGameStatus(0, "Radio Mode | Use /help")
	
	if isInIdleVC {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("✅ Idle radio mode started in default channel!"),
		})
	} else {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("✅ Radio started in current channel! (Not in idle mode)"),
		})
	}
}

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
	
	// Disable idle mode when radio is manually stopped
	client.DisableIdleMode()
	
	streamer.Stop()
	
	s.UpdateGameStatus(0, "/play to add music")
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("✅ Radio stream stopped. Idle mode will resume automatically when everyone leaves the channel."),
	})
}

func floatPtr(f float64) *float64 {
	return &f
}