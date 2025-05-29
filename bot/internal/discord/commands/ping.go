package commands

import (
	"fmt"
	"time"

	"musicbot/internal/socket" // Import your socket package

	"github.com/bwmarrin/discordgo"
)

type PingCommand struct {
	session      *discordgo.Session
	socketClient *socket.Client // Add a field for the socket client
}

// NewPingCommand now accepts the socket client
func NewPingCommand(session *discordgo.Session, socketClient *socket.Client) *PingCommand {
	return &PingCommand{
		session:      session,
		socketClient: socketClient,
	}
}

func (c *PingCommand) Name() string {
	return "ping"
}

func (c *PingCommand) Description() string {
	return "Check bot latency and response time, and downloader status"
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
	botStatus := c.getLatencyStatus(wsLatency)

	// Get downloader status
	downloaderStatus := c.socketClient.GetDownloaderStatus()
	downloaderPingLatency := "N/A"

	// Try to send a ping to the downloader and measure latency
	if c.socketClient.IsConnected() {
		downloaderPingStartTime := time.Now()
		_, err := c.socketClient.SendPingWithResponse()
		if err == nil {
			downloaderPingLatency = fmt.Sprintf("%dms", time.Since(downloaderPingStartTime).Milliseconds())
		} else {
			downloaderPingLatency = fmt.Sprintf("Error: %v", err)
		}
	} else {
		downloaderPingLatency = "Disconnected"
	}

	content := fmt.Sprintf("🏓 **Pong!**\n\n"+
		"📡 **WebSocket Latency:** %dms %s\n"+
		"⚡ **Bot Response Time:** %dms\n"+
		"🤖 **Bot Status:** Online and Ready %s\n"+
		"⬇️ **Downloader Status:** %s (Ping: %s)", // Added downloader status
		wsLatency.Milliseconds(),
		botStatus,
		responseTime.Milliseconds(),
		// You might want to remove this if `botStatus` already includes "Online and Ready"
		// or adjust `botStatus` to be just the latency indicator.
		"", // Placeholder for botStatus if it's already in the string.
		downloaderStatus,
		downloaderPingLatency,
	)

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
