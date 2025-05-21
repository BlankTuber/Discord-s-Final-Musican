package commands

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/discord"
	"quidque.com/discord-musican/internal/logger"
)

// PingCommand handles the /ping command
type PingCommand struct {
	client *discord.Client
}

func NewPingCommand(client *discord.Client) *PingCommand {
	return &PingCommand{
		client: client,
	}
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

func (c *PingCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	startTime := time.Now()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Pinging...",
		},
	})

	latency := time.Since(startTime).Milliseconds()

	heartbeat := s.HeartbeatLatency().Milliseconds()

	// Check downloader connection status
	downloaderStatus := "❌ Disconnected"
	if c.client.DownloaderClient.IsConnected() {
		pingStart := time.Now()
		err := c.client.DownloaderClient.Ping()
		if err == nil {
			downloaderPing := time.Since(pingStart).Milliseconds()
			downloaderStatus = fmt.Sprintf("✅ Connected (%dms)", downloaderPing)
		} else {
			downloaderStatus = fmt.Sprintf("⚠️ Error: %s", err.Error())
		}
	}

	// Count active voice connections
	voiceStatus := "❌ Not connected"
	connectedChannels := c.client.VoiceManager.GetConnectedChannels()
	if len(connectedChannels) > 0 {
		voiceStatus = fmt.Sprintf("✅ Connected to %d channel(s)", len(connectedChannels))
	}

	response := fmt.Sprintf("🏓 Pong!\n"+
		"• **API Latency**: %dms\n"+
		"• **Gateway Ping**: %dms\n"+
		"• **Downloader**: %s\n"+
		"• **Voice**: %s",
		latency, heartbeat, downloaderStatus, voiceStatus)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(response),
	})

	logger.InfoLogger.Printf("Ping command executed - API: %dms, Gateway: %dms", latency, heartbeat)
}

// HelpCommand handles the /help command
type HelpCommand struct {
	client *discord.Client
}

func NewHelpCommand(client *discord.Client) *HelpCommand {
	return &HelpCommand{
		client: client,
	}
}

func (c *HelpCommand) Name() string {
	return "help"
}

func (c *HelpCommand) Description() string {
	return "Show available commands"
}

func (c *HelpCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *HelpCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	// Build the help message
	var response string

	response = "🎵 **Discord Music Bot Help**\n\n"

	response += "**Music Commands:**\n"
	response += "• `/play <url>` - Play a song by URL\n"
	response += "• `/playlist <url> [amount]` - Play a playlist\n"
	response += "• `/search <query> [platform] [count]` - Search for songs\n"
	response += "• `/queue` - Show the current queue\n"
	response += "• `/nowplaying` - Show the currently playing song\n"
	response += "• `/skip` - Skip the current song\n"
	response += "• `/pause` - Pause playback\n"
	response += "• `/start` - Resume paused playback or restart queue\n"
	response += "• `/clear` - Clear the queue\n"
	response += "• `/remove <position>` - Remove a song from the queue\n"
	response += "• `/volume <level>` - Set the volume (0.0 to 1.0)\n\n"

	response += "**Radio Commands:**\n"
	response += "• `/radiostart` - Start radio mode\n"
	response += "• `/radiostop` - Stop radio mode\n"
	response += "• `/radiourl <url>` - Set the radio stream URL\n"
	response += "• `/radiovolume <level>` - Set radio volume\n"
	response += "• `/setidlevc <channel>` - Set default idle voice channel\n\n"

	response += "**Utility Commands:**\n"
	response += "• `/ping` - Check bot's response time\n"
	response += "• `/help` - Show this help menu\n\n"

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(response),
	})
}
