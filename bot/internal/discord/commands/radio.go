package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/discord"
)


type SetDefaultVCCommand struct {
	client *discord.Client
}

func NewSetDefaultVCCommand(client *discord.Client) *SetDefaultVCCommand {
	return &SetDefaultVCCommand{
		client: client,
	}
}

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

func (c *SetDefaultVCCommand) RequiredPermissions() int64 {
	return discordgo.PermissionManageServer
}

func (c *SetDefaultVCCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	channel := options[0].ChannelValue(s)

	if channel.Type != discordgo.ChannelTypeGuildVoice {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Please select a voice channel."),
		})
		return
	}

	c.client.SetDefaultVoiceChannel(i.GuildID, channel.ID)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Default voice channel set to %s", channel.Name)),
	})
}


type RadioURLCommand struct {
	client *discord.Client
}

func NewRadioURLCommand(client *discord.Client) *RadioURLCommand {
	return &RadioURLCommand{
		client: client,
	}
}

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

func (c *RadioURLCommand) RequiredPermissions() int64 {
	return discordgo.PermissionManageServer
}

func (c *RadioURLCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	url := options[0].StringValue()

	c.client.RadioManager.SetStream(url)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("✅ Radio stream URL set to " + url),
	})
}


type RadioVolumeCommand struct {
	client *discord.Client
}

func NewRadioVolumeCommand(client *discord.Client) *RadioVolumeCommand {
	return &RadioVolumeCommand{
		client: client,
	}
}

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

func (c *RadioVolumeCommand) RequiredPermissions() int64 {
	return discordgo.PermissionManageServer
}

func (c *RadioVolumeCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	volume := float32(options[0].FloatValue())

	c.client.RadioManager.SetVolume(volume)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Radio volume set to %.2f", volume)),
	})
}


type RadioStartCommand struct {
	client *discord.Client
}

func NewRadioStartCommand(client *discord.Client) *RadioStartCommand {
	return &RadioStartCommand{
		client: client,
	}
}

func (c *RadioStartCommand) Name() string {
	return "radiostart"
}

func (c *RadioStartCommand) Description() string {
	return "Manually start the radio stream in the current voice channel"
}

func (c *RadioStartCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *RadioStartCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	channelID, err := c.client.GetUserVoiceChannel(i.GuildID, i.Member.User.ID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel to use this command."),
		})
		return
	}

	
	c.client.Mu.RLock()
	isInIdleVC := (channelID == c.client.DefaultVCID && i.GuildID == c.client.DefaultGuildID)
	c.client.Mu.RUnlock()

	
	c.client.DisableIdleMode()

	
	c.client.VoiceManager.StopAllPlayback()

	
	if !c.client.VoiceManager.IsConnectedToChannel(i.GuildID, channelID) {
		err = c.client.RobustJoinVoiceChannel(i.GuildID, channelID)
		if err != nil {
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ Failed to join voice channel: " + err.Error()),
			})
			return
		}
	}

	
	c.client.Mu.Lock()
	c.client.IsInIdleMode = isInIdleVC
	c.client.Mu.Unlock()

	
	c.client.RadioManager.StartInChannel(i.GuildID, channelID)

	s.UpdateGameStatus(0, "Radio Mode | Use /help")

	if isInIdleVC {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("✅ Radio started in idle channel!"),
		})
	} else {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("✅ Radio started in current channel!"),
		})
	}
}


type RadioStopCommand struct {
	client *discord.Client
}

func NewRadioStopCommand(client *discord.Client) *RadioStopCommand {
	return &RadioStopCommand{
		client: client,
	}
}

func (c *RadioStopCommand) Name() string {
	return "radiostop"
}

func (c *RadioStopCommand) Description() string {
	return "Stop the radio stream"
}

func (c *RadioStopCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *RadioStopCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	c.client.Mu.Lock()
	c.client.IsInIdleMode = false
	c.client.Mu.Unlock()

	if !c.client.RadioManager.IsActive() {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Radio is not currently playing."),
		})
		return
	}

	
	c.client.DisableIdleMode()

	c.client.RadioManager.Stop()

	s.UpdateGameStatus(0, "/play to add music")

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("✅ Radio stream stopped. Idle mode will resume automatically when everyone leaves the channel."),
	})
}
