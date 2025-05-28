package commands

import (
	"musicbot/internal/music"
	"musicbot/internal/radio"
	"musicbot/internal/state"
	"musicbot/internal/voice"

	"github.com/bwmarrin/discordgo"
)

type ClearCommand struct {
	voiceManager *voice.Manager
	radioManager *radio.Manager
	musicManager *music.Manager
	stateManager *state.Manager
}

func NewClearCommand(voiceManager *voice.Manager, radioManager *radio.Manager, musicManager *music.Manager, stateManager *state.Manager) *ClearCommand {
	return &ClearCommand{
		voiceManager: voiceManager,
		radioManager: radioManager,
		musicManager: musicManager,
		stateManager: stateManager,
	}
}

func (c *ClearCommand) Name() string {
	return "clear"
}

func (c *ClearCommand) Description() string {
	return "Clear the music queue"
}

func (c *ClearCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *ClearCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	queueItems := c.musicManager.GetQueue()
	if len(queueItems) == 0 {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("📭 Queue is already empty."),
		})
		return err
	}

	// Stop current music
	currentState := c.stateManager.GetBotState()
	if currentState == state.StateDJ {
		c.musicManager.Stop()
	}

	// Clear the queue
	err = c.musicManager.ClearQueue()
	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to clear queue."),
		})
		return err
	}

	// Handle returning to idle state like leave command does
	if c.stateManager.IsInIdleChannel() {
		c.stateManager.SetBotState(state.StateIdle)
		vc := c.voiceManager.GetVoiceConnection()
		if vc != nil {
			c.radioManager.Start(vc)
		}

		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("🗑️ Queue cleared successfully. Radio will continue playing."),
		})
		return err
	}

	// Return to idle channel and start radio
	err = c.voiceManager.LeaveToIdle(i.GuildID)
	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("🗑️ Queue cleared, but failed to return to idle channel."),
		})
		return err
	}

	c.stateManager.SetBotState(state.StateIdle)

	vc := c.voiceManager.GetVoiceConnection()
	if vc != nil {
		c.radioManager.Start(vc)
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("🗑️ Queue cleared successfully. Returned to idle channel and resumed radio."),
	})
	return err
}
