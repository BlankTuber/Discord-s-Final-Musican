package commands

import (
	"musicbot/internal/radio"
	"musicbot/internal/state"
	"musicbot/internal/voice"

	"github.com/bwmarrin/discordgo"
)

type LeaveCommand struct {
	voiceManager *voice.Manager
	radioManager *radio.Manager
	stateManager *state.Manager
}

func NewLeaveCommand(voiceManager *voice.Manager, radioManager *radio.Manager, stateManager *state.Manager) *LeaveCommand {
	return &LeaveCommand{
		voiceManager: voiceManager,
		radioManager: radioManager,
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

	c.radioManager.Stop()

	if c.stateManager.IsInIdleChannel() {
		c.stateManager.SetBotState(state.StateIdle)
		vc := c.voiceManager.GetVoiceConnection()
		if vc != nil {
			c.radioManager.Start(vc)
		}

		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("✅ This is the idle channel. Radio will continue playing."),
		})
		return err
	}

	err = c.voiceManager.LeaveToIdle(i.GuildID)
	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to return to idle channel."),
		})
		return err
	}

	c.stateManager.SetBotState(state.StateIdle)

	vc := c.voiceManager.GetVoiceConnection()
	if vc != nil {
		c.radioManager.Start(vc)
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("✅ Returned to idle channel and resumed radio."),
	})
	return err
}
