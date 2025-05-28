package commands

import (
	"musicbot/internal/music"
	"musicbot/internal/state"

	"github.com/bwmarrin/discordgo"
)

type SkipCommand struct {
	musicManager *music.Manager
	stateManager *state.Manager
}

func NewSkipCommand(musicManager *music.Manager, stateManager *state.Manager) *SkipCommand {
	return &SkipCommand{
		musicManager: musicManager,
		stateManager: stateManager,
	}
}

func (c *SkipCommand) Name() string {
	return "skip"
}

func (c *SkipCommand) Description() string {
	return "Skip the current song"
}

func (c *SkipCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *SkipCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	if c.stateManager.GetBotState() != state.StateDJ {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Not currently playing music."),
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

	if !c.musicManager.IsPlaying() {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No song is currently playing."),
		})
		return err
	}

	upcoming := c.musicManager.GetUpcoming(1)
	if len(upcoming) == 0 {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("⏭️ Skipped current song. No more songs in queue."),
		})
	} else {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("⏭️ Skipped to next song."),
		})
	}

	c.musicManager.Stop()

	return err
}
