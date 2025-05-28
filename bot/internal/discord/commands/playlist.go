package commands

import (
	"fmt"
	"musicbot/internal/music"
	"musicbot/internal/radio"
	"musicbot/internal/state"
	"musicbot/internal/voice"
	"time"

	"github.com/bwmarrin/discordgo"
)

type PlaylistCommand struct {
	voiceManager *voice.Manager
	radioManager *radio.Manager
	musicManager *music.Manager
	stateManager *state.Manager
}

func NewPlaylistCommand(voiceManager *voice.Manager, radioManager *radio.Manager, musicManager *music.Manager, stateManager *state.Manager) *PlaylistCommand {
	return &PlaylistCommand{
		voiceManager: voiceManager,
		radioManager: radioManager,
		musicManager: musicManager,
		stateManager: stateManager,
	}
}

func (c *PlaylistCommand) Name() string {
	return "playlist"
}

func (c *PlaylistCommand) Description() string {
	return "Play a playlist from URL"
}

func (c *PlaylistCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "url",
			Description: "URL of the playlist to play",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "limit",
			Description: "Maximum number of songs to download (default: 20, max: 50)",
			Required:    false,
			MinValue:    func() *float64 { v := 1.0; return &v }(),
			MaxValue:    50,
		},
	}
}

func (c *PlaylistCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	options := i.ApplicationCommandData().Options
	url := options[0].StringValue()
	userID := i.Member.User.ID

	limit := 20
	if len(options) > 1 && options[1].IntValue() > 0 {
		providedLimit := int(options[1].IntValue())
		if providedLimit > 50 {
			limit = 50
		} else {
			limit = providedLimit
		}
	}

	userVS, err := s.State.VoiceState(i.GuildID, userID)
	if err != nil || userVS == nil || userVS.ChannelID == "" {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel."),
		})
		return err
	}

	userChannelID := userVS.ChannelID
	currentChannelID := c.stateManager.GetCurrentChannel()

	if currentChannelID != "" && currentChannelID != userChannelID {
		currentBotState := c.stateManager.GetBotState()

		if currentBotState == state.StateDJ && c.musicManager.IsPlaying() {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ Bot is currently playing music in another channel."),
			})
			return err
		}

		c.radioManager.Stop()
		c.musicManager.Stop()

		err = c.voiceManager.JoinUser(i.GuildID, userID)
		if err != nil {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ Failed to join your voice channel."),
			})
			return err
		}

		time.Sleep(500 * time.Millisecond)

		if currentBotState == state.StateRadio {
			vc := c.voiceManager.GetVoiceConnection()
			if vc != nil {
				c.radioManager.Start(vc)
			}
		}
	} else if currentChannelID == "" {
		err = c.voiceManager.JoinUser(i.GuildID, userID)
		if err != nil {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ Failed to join your voice channel."),
			})
			return err
		}
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("📜 Starting playlist download from: %s\n⏳ Downloading up to %d songs. Songs will be added to queue as they download...", url, limit)),
	})
	if err != nil {
		return err
	}

	go func() {
		err := c.musicManager.RequestPlaylist(url, userID, limit)
		if err != nil {
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr(fmt.Sprintf("❌ Failed to request playlist: %v", err)),
			})
		}
	}()

	return nil
}
