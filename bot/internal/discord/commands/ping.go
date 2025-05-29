package commands

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
)

type PingCommand struct {
	session *discordgo.Session
}

func NewPingCommand(session *discordgo.Session) *PingCommand {
	return &PingCommand{
		session: session,
	}
}

func (c *PingCommand) Name() string {
	return "ping"
}

func (c *PingCommand) Description() string {
	return "Check bot latency and response time"
}

func (c *PingCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *PingCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	startTime := time.Now()

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	responseTime := time.Since(startTime)

	wsLatency := s.HeartbeatLatency()

	status := c.getLatencyStatus(wsLatency)

	content := fmt.Sprintf("🏓 **Pong!**\n\n"+
		"📡 **WebSocket Latency:** %dms %s\n"+
		"⚡ **Response Time:** %dms\n"+
		"🤖 **Bot Status:** Online and Ready",
		wsLatency.Milliseconds(),
		status,
		responseTime.Milliseconds())

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
	return err
}

func (c *PingCommand) getLatencyStatus(latency time.Duration) string {
	ms := latency.Milliseconds()

	if ms < 100 {
		return "🟢 (Excellent)"
	} else if ms < 200 {
		return "🟡 (Good)"
	} else if ms < 500 {
		return "🟠 (Fair)"
	} else {
		return "🔴 (Poor)"
	}
}
