package commands

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/discord"
	"quidque.com/discord-musican/internal/logger"
)

const (
	DefaultPlaylistMax    = 7
	DefaultPlaylistMaxOpt = 15
	DefaultMaxDuration    = 600
	DefaultMaxSize        = 150
	MaxRetryAttempts      = 2
)

type PlaylistCommand struct {
	client *discord.Client
}

func NewPlaylistCommand(client *discord.Client) *PlaylistCommand {
	return &PlaylistCommand{
		client: client,
	}
}

func (c *PlaylistCommand) Name() string {
	return "playlist"
}

func (c *PlaylistCommand) Description() string {
	return "Play a playlist from URL"
}

func (c *PlaylistCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "url",
			Description: "URL of the playlist to play",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "amount",
			Description: "Number of songs to download (1-20)",
			Required:    false,
			MinValue:    floatPtr(1),
			MaxValue:    float64(DefaultPlaylistMaxOpt),
		},
	}
}

func (c *PlaylistCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	url := options[0].StringValue()

	maxItems := DefaultPlaylistMax
	if len(options) > 1 {
		maxItems = int(options[1].IntValue())
	}

	channelID, err := c.client.GetUserVoiceChannel(i.GuildID, i.Member.User.ID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel to use this command."),
		})
		return
	}

	c.client.DisableIdleMode()

	err = c.client.JoinVoiceChannel(i.GuildID, channelID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to join voice channel: " + err.Error()),
		})
		return
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("⏳ Processing playlist... This may take a while."),
	})

	logger.InfoLogger.Printf("Getting playlist info for URL: %s", url)

	// First, get basic playlist info to know what we're dealing with
	playlistTitle, totalTracks, err := c.client.DownloaderClient.GetPlaylistInfo(url, maxItems)
	if err != nil {
		// Try as a single track if playlist info fails
		logger.InfoLogger.Printf("Failed to get playlist info, trying as single track: %v", err)

		track, singleErr := c.client.DownloaderClient.DownloadAudio(url, DefaultMaxDuration, DefaultMaxSize, false)
		if singleErr == nil && track != nil && track.FilePath != "" {
			track.Requester = i.Member.User.Username
			track.RequestedAt = time.Now().Unix()

			c.client.QueueManager.AddTrack(i.GuildID, track)

			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr(fmt.Sprintf("✅ Added single track to queue: **%s**", track.Title)),
			})
			return
		}

		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to process URL: %s", err.Error())),
		})
		return
	}

	if totalTracks == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No playable tracks found in the playlist."),
		})
		return
	}

	// Update the message with initial info
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("⏳ Processing **%s**\nFound %d tracks, downloading...", playlistTitle, totalTracks)),
	})

	// Start downloading the playlist items incrementally
	var successCount int
	var failCount int
	var mu sync.Mutex

	// Start a goroutine to update the message periodically
	stopUpdates := make(chan struct{})
	defer close(stopUpdates)

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				mu.Lock()
				current := successCount
				failed := failCount
				mu.Unlock()

				if current+failed > 0 {
					progress := fmt.Sprintf("⏳ Downloading **%s**\nProgress: %d/%d tracks downloaded (%d failed)",
						playlistTitle, current, totalTracks, failed)

					s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
						Content: stringPtr(progress),
					})
				}
			case <-stopUpdates:
				return
			}
		}
	}()

	// Process each track individually
	failedTracks := make([]string, 0)

	for idx := 0; idx < totalTracks; idx++ {
		var track interface{} = nil
		var err error

		// Try up to MaxRetryAttempts times
		for attempt := 0; attempt < MaxRetryAttempts; attempt++ {
			// Add a small delay between retries to prevent rate limiting
			if attempt > 0 {
				time.Sleep(time.Duration(attempt) * time.Second)
				logger.InfoLogger.Printf("Retry attempt %d for track %d", attempt+1, idx+1)
			}

			// Download the track
			track, err = c.client.DownloaderClient.DownloadPlaylistItem(url, idx, DefaultMaxDuration, DefaultMaxSize, false)
			if err != nil {
				// Log the error but continue with retries
				logger.ErrorLogger.Printf("Attempt %d: Failed to download playlist item %d: %v",
					attempt+1, idx+1, err)
				continue
			}

			// Success, break out of retry loop
			break
		}

		// Check if all attempts failed
		if err != nil || track == nil {
			mu.Lock()
			failCount++
			failedTracks = append(failedTracks, fmt.Sprintf("Track #%d", idx+1))
			mu.Unlock()
			logger.ErrorLogger.Printf("All attempts failed for track %d: %v", idx+1, err)
			continue
		}

		// Convert the interface to the correct type
		audioTrack, ok := track.(*audio.Track)
		if !ok || audioTrack.FilePath == "" || !fileExists(audioTrack.FilePath) {
			mu.Lock()
			failCount++
			failedTracks = append(failedTracks, fmt.Sprintf("Track #%d", idx+1))
			mu.Unlock()
			logger.ErrorLogger.Printf("Invalid track data for item %d", idx+1)
			continue
		}

		// Set track metadata
		audioTrack.Requester = i.Member.User.Username
		audioTrack.RequestedAt = time.Now().Unix()

		// Add to queue - make sure to use the correct guild ID
		c.client.QueueManager.AddTrack(i.GuildID, audioTrack)

		mu.Lock()
		successCount++
		mu.Unlock()

		// If this is the first successful track, notify about playback starting
		if successCount == 1 {
			c.client.StartPlayback(i.GuildID)
		}
	}

	// Send final message
	finalMessage := ""
	if successCount > 0 {
		finalMessage = fmt.Sprintf("✅ Added %d/%d tracks from **%s** to the queue!",
			successCount, totalTracks, playlistTitle)
	} else {
		finalMessage = fmt.Sprintf("❌ Failed to download any tracks from **%s**!", playlistTitle)
	}

	if len(failedTracks) > 0 {
		if len(failedTracks) <= 5 {
			failList := strings.Join(failedTracks, "\n• ")
			finalMessage += fmt.Sprintf("\n\nFailed to download:\n• %s", failList)
		} else {
			finalMessage += fmt.Sprintf("\n\n%d tracks failed to download.", len(failedTracks))
		}
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(finalMessage),
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
