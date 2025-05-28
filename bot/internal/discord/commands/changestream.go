package commands

import (
	"musicbot/internal/config"
	"musicbot/internal/radio"
	"musicbot/internal/voice"

	"github.com/bwmarrin/discordgo"
)

type ChangeStreamCommand struct {
	voiceManager *voice.Manager
	radioManager *radio.Manager
	dbManager    *config.DatabaseManager
}

func NewChangeStreamCommand(voiceManager *voice.Manager, radioManager *radio.Manager, dbManager *config.DatabaseManager) *ChangeStreamCommand {
	return &ChangeStreamCommand{
		voiceManager: voiceManager,
		radioManager: radioManager,
		dbManager:    dbManager,
	}
}

func (c *ChangeStreamCommand) Name() string {
	return "changestream"
}

func (c *ChangeStreamCommand) Description() string {
	return "Change the radio stream"
}

func (c *ChangeStreamCommand) Options() []*discordgo.ApplicationCommandOption {
	streamChoices := []*discordgo.ApplicationCommandOptionChoice{
		{Name: "listen.moe", Value: "listen.moe"},
		{Name: "listen.moe (kpop)", Value: "listen.moe (kpop)"},
		{Name: "lofi", Value: "lofi"},
	}

	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "stream",
			Description: "Select a radio stream",
			Required:    true,
			Choices:     streamChoices,
		},
	}
}

func (c *ChangeStreamCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	streamName := i.ApplicationCommandData().Options[0].StringValue()

	if !c.radioManager.IsValidStream(streamName) {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Invalid stream selection."),
		})
		return err
	}

	c.radioManager.Stop()

	err = c.radioManager.ChangeStream(streamName)
	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to change stream."),
		})
		return err
	}

	if c.dbManager != nil {
		streamURL := ""
		streams := config.GetDefaultStreams()
		for _, stream := range streams {
			if stream.Name == streamName {
				streamURL = stream.URL
				break
			}
		}
		if streamURL != "" {
			c.dbManager.SaveStream(streamURL)
		}
	}

	vc := c.voiceManager.GetVoiceConnection()
	if vc != nil {
		c.radioManager.Start(vc)
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("✅ Changed radio stream to " + streamName),
	})
	return err
}
