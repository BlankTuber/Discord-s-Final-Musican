package commands

import (
	"fmt"
	"musicbot/internal/music"
	"musicbot/internal/state"

	"github.com/bwmarrin/discordgo"
)

type QueueCommand struct {
	musicManager *music.Manager
	stateManager *state.Manager
}

func NewQueueCommand(musicManager *music.Manager, stateManager *state.Manager) *QueueCommand {
	return &QueueCommand{
		musicManager: musicManager,
		stateManager: stateManager,
	}
}

func (c *QueueCommand) Name() string {
	return "queue"
}

func (c *QueueCommand) Description() string {
	return "Show the current music queue"
}

func (c *QueueCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *QueueCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: c.generateQueueMessage(),
		},
	})
	return err
}

func (c *QueueCommand) generateQueueMessage() string {
	currentSong := c.musicManager.GetCurrentSong()
	upcoming := c.musicManager.GetUpcoming(10)

	if currentSong == nil && len(upcoming) == 0 {
		return "📭 Queue is empty. Use `/play` to add songs!"
	}

	message := "🎵 **Music Queue**\n\n"

	if currentSong != nil {
		duration := c.formatDuration(currentSong.Duration)
		message += fmt.Sprintf("🎧 **Now Playing:**\n**%s** - %s (%s)\n\n",
			currentSong.Title, currentSong.Artist, duration)
	}

	if len(upcoming) > 0 {
		message += "📋 **Up Next:**\n"
		for i, song := range upcoming {
			duration := c.formatDuration(song.Duration)
			message += fmt.Sprintf("**%d.** %s - %s (%s)\n",
				i+1, song.Title, song.Artist, duration)
		}
	}

	totalSongs := len(upcoming)
	if currentSong != nil {
		totalSongs++
	}

	message += fmt.Sprintf("\n📊 **Total songs:** %d", totalSongs)

	return message
}

func (c *QueueCommand) formatDuration(seconds int) string {
	if seconds <= 0 {
		return "Unknown"
	}

	minutes := seconds / 60
	secs := seconds % 60
	return fmt.Sprintf("%d:%02d", minutes, secs)
}
