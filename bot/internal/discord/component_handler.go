package discord

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/logger"
)

// handleComponentInteraction handles all component interactions like buttons
func (c *Client) handleComponentInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Only process component interactions
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}
	
	// Register activity
	c.StartActivity()
	
	// Extract the component type and custom ID
	customID := i.MessageComponentData().CustomID
	
	// Get the handler prefix (everything before the first colon)
	var handlerPrefix string
	if idx := strings.Index(customID, ":"); idx != -1 {
		handlerPrefix = customID[:idx]
	} else {
		handlerPrefix = customID
	}
	
	// Find the appropriate handler
	handler, exists := c.componentHandlers[handlerPrefix]
	if !exists {
		logger.WarnLogger.Printf("No handler for component with prefix: %s", handlerPrefix)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Unknown component interaction.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	
	// Call the handler
	handler(s, i)
}
