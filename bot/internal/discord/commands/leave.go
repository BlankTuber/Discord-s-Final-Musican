package commands

import (
	"musicbot/internal/music"
	"musicbot/internal/radio"
	"musicbot/internal/state"
	"musicbot/internal/voice"
	"time"

	"github.com/bwmarrin/discordgo"
)

type LeaveCommand struct {
	voiceManager *voice.Manager
	radioManager *radio.Manager
	musicManager *music.Manager
	stateManager *state.Manager
}

func NewLeaveCommand(voiceManager *voice.Manager, radioManager *radio.Manager, musicManager *music.Manager, stateManager *state.Manager) *LeaveCommand {
	return &LeaveCommand{
		voiceManager: voiceManager,
		radioManager: radioManager,
		musicManager: musicManager,
		stateManager: stateManager,
	}
}

func (c *LeaveCommand) Name() string {
	return "leave"
}

func (c *LeaveCommand) Description() string {
	return "Leave current voice channel"
}

func (c *LeaveCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *LeaveCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	currentState := c.stateManager.GetBotState()

	c.stateManager.SetManualOperationActive(true)
	defer c.stateManager.SetManualOperationActive(false)

	c.musicManager.ExecuteWithDisabledHandlers(func() {
		if currentState == state.StateDJ {
			c.musicManager.Stop()
		} else {
			c.radioManager.Stop()
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
			Content: stringPtr("❌ Failed to return to idle channel."),
		})
		return err
	}

	if c.stateManager.IsInIdleChannel() {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("✅ This is the idle channel. Radio will continue playing."),
		})
	} else {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("✅ Returned to idle channel and resumed radio."),
		})
	}

	return err
}
