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

type PlayCommand struct {
	voiceManager *voice.Manager
	radioManager *radio.Manager
	musicManager *music.Manager
	stateManager *state.Manager
}

func NewPlayCommand(voiceManager *voice.Manager, radioManager *radio.Manager, musicManager *music.Manager, stateManager *state.Manager) *PlayCommand {
	return &PlayCommand{
		voiceManager: voiceManager,
		radioManager: radioManager,
		musicManager: musicManager,
		stateManager: stateManager,
	}
}

func (c *PlayCommand) Name() string {
	return "play"
}

func (c *PlayCommand) Description() string {
	return "Play a song from URL"
}

func (c *PlayCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "url",
			Description: "URL of the song to play",
			Required:    true,
		},
	}
}

func (c *PlayCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	url := i.ApplicationCommandData().Options[0].StringValue()
	userID := i.Member.User.ID

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
		Content: stringPtr(fmt.Sprintf("🎵 Downloading song from: %s\n⏳ This may take a moment...", url)),
	})
	if err != nil {
		return err
	}

	err = c.musicManager.RequestSong(url, userID)
	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to request song: %v", err)),
		})
		return err
	}

	return nil
}
