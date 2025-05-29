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
	if len(queueItems) == 0 && !c.musicManager.IsPlaying() {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("📭 Queue is already empty."),
		})
		return err
	}

	if c.musicManager.HasActiveDownloads() {
		pendingCount := c.musicManager.GetPendingDownloads()
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("⏳ Cannot clear queue while %d songs are downloading. Please wait for downloads to complete.", pendingCount)),
		})
		return err
	}

	c.radioManager.Stop()
	c.musicManager.Stop()

	time.Sleep(1 * time.Second)

	err = c.musicManager.ClearQueue()
	if err != nil {
		if err.Error() == "cannot clear queue while downloads are in progress" {
			pendingCount := c.musicManager.GetPendingDownloads()
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr(fmt.Sprintf("⏳ Cannot clear queue while %d songs are downloading. Please wait for downloads to complete.", pendingCount)),
			})
		} else {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ Failed to clear queue."),
			})
		}
		return err
	}

	time.Sleep(500 * time.Millisecond)

	if c.stateManager.IsInIdleChannel() {
		c.stateManager.SetBotState(state.StateIdle)

		time.Sleep(500 * time.Millisecond)

		vc := c.voiceManager.GetVoiceConnection()
		if vc != nil && !c.radioManager.IsPlaying() {
			c.radioManager.Start(vc)
		}

		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("🗑️ Queue cleared successfully. Radio will continue playing."),
		})
		return err
	}

	err = c.voiceManager.LeaveToIdle(i.GuildID)
	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("🗑️ Queue cleared, but failed to return to idle channel."),
		})
		return err
	}

	c.stateManager.SetBotState(state.StateIdle)

	time.Sleep(500 * time.Millisecond)

	vc := c.voiceManager.GetVoiceConnection()
	if vc != nil && !c.radioManager.IsPlaying() {
		c.radioManager.Start(vc)
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("🗑️ Queue cleared successfully. Returned to idle channel and resumed radio."),
	})
	return err
}
