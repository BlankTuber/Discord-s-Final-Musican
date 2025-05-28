package commands

import (
	"musicbot/internal/music"
	"musicbot/internal/state"

	"github.com/bwmarrin/discordgo"
)

type ClearCommand struct {
	musicManager *music.Manager
	stateManager *state.Manager
}

func NewClearCommand(musicManager *music.Manager, stateManager *state.Manager) *ClearCommand {
	return &ClearCommand{
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

	currentState := c.stateManager.GetBotState()
	if currentState == state.StateDJ {
		c.musicManager.Stop()
	}

	err = c.musicManager.ClearQueue()
	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to clear queue."),
		})
		return err
	}

	c.stateManager.SetBotState(state.StateIdle)

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("🗑️ Queue cleared successfully."),
	})
	return err
}
