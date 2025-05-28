package commands

import (
	"fmt"
	"musicbot/internal/music"
	"musicbot/internal/radio"
	"musicbot/internal/state"

	"github.com/bwmarrin/discordgo"
)

type NowPlayingCommand struct {
	musicManager *music.Manager
	radioManager *radio.Manager
	stateManager *state.Manager
}

func NewNowPlayingCommand(musicManager *music.Manager, radioManager *radio.Manager, stateManager *state.Manager) *NowPlayingCommand {
	return &NowPlayingCommand{
		musicManager: musicManager,
		radioManager: radioManager,
		stateManager: stateManager,
	}
}

func (c *NowPlayingCommand) Name() string {
	return "nowplaying"
}

func (c *NowPlayingCommand) Description() string {
	return "Show what's currently playing"
}

func (c *NowPlayingCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *NowPlayingCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	message := c.generateNowPlayingMessage()

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
		},
	})
	return err
}

func (c *NowPlayingCommand) generateNowPlayingMessage() string {
	currentState := c.stateManager.GetBotState()

	switch currentState {
	case state.StateDJ:
		currentSong := c.musicManager.GetCurrentSong()
		if currentSong == nil {
			return "🎵 **DJ Mode** - No song currently playing"
		}

		duration := c.formatDuration(currentSong.Duration)
		message := fmt.Sprintf("🎧 **Now Playing:**\n**%s** - %s\n⏱️ Duration: %s\n👤 Requested by: <@%s>",
			currentSong.Title, currentSong.Artist, duration, currentSong.RequestedBy)

		upcoming := c.musicManager.GetUpcoming(3)
		if len(upcoming) > 0 {
			message += "\n\n📋 **Up Next:**\n"
			for i, song := range upcoming {
				songDuration := c.formatDuration(song.Duration)
				message += fmt.Sprintf("**%d.** %s - %s (%s)\n",
					i+1, song.Title, song.Artist, songDuration)
			}
		}

		return message

	case state.StateRadio:
		streamName := c.getStreamName()
		if streamName != "" {
			return fmt.Sprintf("📻 **Radio Mode** - Playing: %s", streamName)
		}
		return "📻 **Radio Mode** - Playing radio stream"

	case state.StateIdle:
		streamName := c.getStreamName()
		if streamName != "" {
			return fmt.Sprintf("😴 **Idle Mode** - Playing: %s", streamName)
		}
		return "😴 **Idle Mode** - Playing radio stream"

	default:
		return "❓ **Unknown State** - Not sure what's playing"
	}
}

func (c *NowPlayingCommand) getStreamName() string {
	currentStreamURL := c.stateManager.GetRadioStream()

	streamNames := map[string]string{
		"https://listen.moe/stream":                      "listen.moe",
		"https://listen.moe/kpop/stream":                 "listen.moe (kpop)",
		"https://streams.ilovemusic.de/iloveradio17.mp3": "lofi",
	}

	if name, exists := streamNames[currentStreamURL]; exists {
		return name
	}

	return ""
}

func (c *NowPlayingCommand) formatDuration(seconds int) string {
	if seconds <= 0 {
		return "Unknown"
	}

	minutes := seconds / 60
	secs := seconds % 60
	return fmt.Sprintf("%d:%02d", minutes, secs)
}
