package discord

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/logger"
)

type PingCommand struct{}

func registerPingCommand(registry *CommandRegistry) {
	registry.Register(&PingCommand{})
}

func (c *PingCommand) Name() string {
	return "ping"
}

func (c *PingCommand) Description() string {
	return "Check the bot's response time"
}

func (c *PingCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *PingCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	startTime := time.Now()
	
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Pinging...",
		},
	})
	
	latency := time.Since(startTime).Milliseconds()
	
	heartbeat := s.HeartbeatLatency().Milliseconds()
	
	response := fmt.Sprintf("🏓 Pong!\n"+
		"• **API Latency**: %dms\n"+
		"• **Gateway Ping**: %dms", 
		latency, heartbeat)
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(response),
	})
	
	logger.InfoLogger.Printf("Ping command executed - API: %dms, Gateway: %dms", latency, heartbeat)
}