package commands

import (
	"fmt"
	"time"

	"musicbot/internal/socket"

	"github.com/bwmarrin/discordgo"
)

type PingCommand struct {
	session      *discordgo.Session
	socketClient *socket.Client
}

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

	downloaderStatus := c.socketClient.GetDownloaderStatus()
	downloaderPingLatency := "N/A"
	downloaderError := ""

	if c.socketClient.IsConnected() {
		downloaderPingStartTime := time.Now()
		_, err := c.socketClient.SendPingWithResponse()
		if err == nil {
			downloaderPingLatency = fmt.Sprintf("%dms", time.Since(downloaderPingStartTime).Milliseconds())
		} else {
			downloaderPingLatency = "Failed"
			downloaderError = err.Error()
		}
	} else {
		downloaderPingLatency = "Disconnected"
		downloaderError = "Not connected to downloader service"
	}

	var content string
	if downloaderError != "" {
		content = fmt.Sprintf("🏓 **Pong!**\n\n"+
			"📡 **WebSocket Latency:** %dms %s\n"+
			"⚡ **Bot Response Time:** %dms\n"+
			"🤖 **Bot Status:** Online and Ready\n"+
			"⬇️ **Downloader Status:** %s\n"+
			"📶 **Downloader Ping:** %s\n"+
			"❌ **Downloader Error:** %s",
			wsLatency.Milliseconds(),
			botStatus,
			responseTime.Milliseconds(),
			downloaderStatus,
			downloaderPingLatency,
			downloaderError,
		)
	} else {
		content = fmt.Sprintf("🏓 **Pong!**\n\n"+
			"📡 **WebSocket Latency:** %dms %s\n"+
			"⚡ **Bot Response Time:** %dms\n"+
			"🤖 **Bot Status:** Online and Ready\n"+
			"⬇️ **Downloader Status:** %s\n"+
			"📶 **Downloader Ping:** %s",
			wsLatency.Milliseconds(),
			botStatus,
			responseTime.Milliseconds(),
			downloaderStatus,
			downloaderPingLatency,
		)
	}

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
