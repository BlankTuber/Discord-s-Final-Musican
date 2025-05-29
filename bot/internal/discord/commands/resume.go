package commands

import (
	"musicbot/internal/music"
	"musicbot/internal/radio"
	"musicbot/internal/state"
	"musicbot/internal/voice"
	"time"

	"github.com/bwmarrin/discordgo"
)

type ResumeCommand struct {
	voiceManager *voice.Manager
	radioManager *radio.Manager
	musicManager *music.Manager
	stateManager *state.Manager
}

func NewResumeCommand(voiceManager *voice.Manager, radioManager *radio.Manager, musicManager *music.Manager, stateManager *state.Manager) *ResumeCommand {
	return &ResumeCommand{
		voiceManager: voiceManager,
		radioManager: radioManager,
		musicManager: musicManager,
		stateManager: stateManager,
	}
}

func (c *ResumeCommand) Name() string {
	return "resume"
}

func (c *ResumeCommand) Description() string {
	return "Resume paused music in your voice channel"
}

func (c *ResumeCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *ResumeCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	currentSong := c.musicManager.GetCurrentSong()
	queueItems := c.musicManager.GetQueue()

	if currentSong == nil && len(queueItems) == 0 {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No music queue available to resume."),
		})
		return err
	}

	if c.musicManager.IsPlaying() {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Music is already playing."),
		})
		return err
	}

	userVS, err := s.State.VoiceState(i.GuildID, i.Member.User.ID)
	if err != nil || userVS == nil || userVS.ChannelID == "" {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel."),
		})
		return err
	}

	userChannelID := userVS.ChannelID
	currentChannelID := c.stateManager.GetCurrentChannel()

	c.stateManager.SetManualOperationActive(true)
	defer c.stateManager.SetManualOperationActive(false)

	c.musicManager.ExecuteWithDisabledHandlers(func() {
		c.radioManager.Stop()

		if currentChannelID != userChannelID {
			time.Sleep(500 * time.Millisecond)

			err = c.voiceManager.JoinUser(i.GuildID, i.Member.User.ID)
			if err != nil {
				return
			}

			time.Sleep(500 * time.Millisecond)
		}

		c.stateManager.SetBotState(state.StateDJ)

		if c.musicManager.IsPaused() {
			err = c.musicManager.Resume()
		} else {
			vc := c.voiceManager.GetVoiceConnection()
			if vc != nil {
				err = c.musicManager.Start(vc)
			}
		}
	})

	if err != nil {
		if err.Error() == "user not in voice channel" {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ You need to be in a voice channel."),
			})
		} else if err.Error() == "already in user's channel" {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ Failed to resume music."),
			})
		} else {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ Failed to resume music."),
			})
		}
		return err
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("▶️ Music resumed!"),
	})
	return err
}
