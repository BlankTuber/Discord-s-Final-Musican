package commands

import (
	"musicbot/internal/music"
	"musicbot/internal/radio"
	"musicbot/internal/state"
	"musicbot/internal/voice"
	"time"

	"github.com/bwmarrin/discordgo"
)

type PauseCommand struct {
	voiceManager *voice.Manager
	radioManager *radio.Manager
	musicManager *music.Manager
	stateManager *state.Manager
}

func NewPauseCommand(voiceManager *voice.Manager, radioManager *radio.Manager, musicManager *music.Manager, stateManager *state.Manager) *PauseCommand {
	return &PauseCommand{
		voiceManager: voiceManager,
		radioManager: radioManager,
		musicManager: musicManager,
		stateManager: stateManager,
	}
}

func (c *PauseCommand) Name() string {
	return "pause"
}

func (c *PauseCommand) Description() string {
	return "Pause music and switch to idle mode"
}

func (c *PauseCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *PauseCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	currentState := c.stateManager.GetBotState()

	if currentState != state.StateDJ {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No music is currently playing."),
		})
		return err
	}

	if !c.musicManager.IsPlaying() {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No song is currently playing."),
		})
		return err
	}

	if c.musicManager.IsPaused() {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Music is already paused."),
		})
		return err
	}

	currentSong := c.musicManager.GetCurrentSong()
	if currentSong == nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No song is currently playing."),
		})
		return err
	}

	c.stateManager.SetManualOperationActive(true)
	defer c.stateManager.SetManualOperationActive(false)

	c.musicManager.ExecuteWithDisabledHandlers(func() {
		err = c.musicManager.Pause()
		if err != nil {
			return
		}

		time.Sleep(500 * time.Millisecond)

		if c.stateManager.IsInIdleChannel() {
			c.stateManager.SetBotState(state.StateIdle)

			time.Sleep(500 * time.Millisecond)

			vc := c.voiceManager.GetVoiceConnection()
			if vc != nil && !c.radioManager.IsPlaying() {
				c.radioManager.Start(vc)
			}
		} else {
			err = c.voiceManager.LeaveToIdle(i.GuildID)
			if err != nil {
				return
			}

			c.stateManager.SetBotState(state.StateIdle)

			time.Sleep(500 * time.Millisecond)

			vc := c.voiceManager.GetVoiceConnection()
			if vc != nil && !c.radioManager.IsPlaying() {
				c.radioManager.Start(vc)
			}
		}
	})

	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to pause music."),
		})
		return err
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("⏸️ Music paused. Use `/resume` to continue playing."),
	})
	return err
}
