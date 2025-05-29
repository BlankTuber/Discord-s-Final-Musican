package commands

import (
	"fmt"
	"musicbot/internal/config"
	"musicbot/internal/state"

	"github.com/bwmarrin/discordgo"
)

type VolumeCommand struct {
	stateManager *state.Manager
	dbManager    *config.DatabaseManager
}

func NewVolumeCommand(stateManager *state.Manager, dbManager *config.DatabaseManager) *VolumeCommand {
	return &VolumeCommand{
		stateManager: stateManager,
		dbManager:    dbManager,
	}
}

func (c *VolumeCommand) Name() string {
	return "volume"
}

func (c *VolumeCommand) Description() string {
	return "Set the playback volume"
}

func (c *VolumeCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "level",
			Description: "Volume level (1-100)",
			Required:    false,
			MinValue:    func() *float64 { v := 1.0; return &v }(),
			MaxValue:    100,
		},
	}
}

func (c *VolumeCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	options := i.ApplicationCommandData().Options

	if len(options) == 0 {
		currentVolume := c.stateManager.GetVolume()
		percentage := int(currentVolume * 1000)

		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("🔊 Current volume: %d%%", percentage)),
		})
		return err
	}

	level := int(options[0].IntValue())
	volumeFloat := float32(level) / 1000.0

	if volumeFloat < 0.01 {
		volumeFloat = 0.01
	} else if volumeFloat > 0.1 {
		volumeFloat = 0.1
	}

	c.stateManager.SetVolume(volumeFloat)

	if c.dbManager != nil {
		err = c.dbManager.SaveVolume(volumeFloat)
		if err != nil {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr(fmt.Sprintf("🔊 Volume set to %d%% but failed to save to database.", level)),
			})
			return err
		}
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("🔊 Volume set to %d%%", level)),
	})
	return err
}
