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

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("⏳ Processing playlist... This may take a while."),
	})

	logger.InfoLogger.Printf("Getting playlist info for URL: %s", url)

	playlistTitle, totalTracks, err := c.client.DownloaderClient.GetPlaylistInfo(url, maxItems)
	if err != nil {
		logger.InfoLogger.Printf("Failed to get playlist info, trying as single track: %v", err)

		track, singleErr := c.client.DownloaderClient.DownloadAudio(url, DefaultMaxDuration, DefaultMaxSize, false)
		if singleErr == nil && track != nil && track.FilePath != "" {
			track.Requester = i.Member.User.Username
			track.RequestedAt = time.Now().Unix()

			err = c.client.RobustJoinVoiceChannel(i.GuildID, channelID)
			if err != nil {
				c.client.QueueManager.AddTrack(i.GuildID, track)
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: stringPtr(fmt.Sprintf("⚠️ Added **%s** to queue but couldn't join voice. Use /start to begin playback.", track.Title)),
				})
				return
			}

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

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("⏳ Processing **%s**\nFound %d tracks, downloading...", playlistTitle, totalTracks)),
	})

	var successCount int
	var failCount int
	var mu sync.Mutex
	var firstTrackAdded bool = false
	var joinedVC bool = false

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
				joined := joinedVC
				mu.Unlock()

				if current+failed > 0 {
					progress := fmt.Sprintf("⏳ Downloading **%s**\nProgress: %d/%d tracks downloaded (%d failed)",
						playlistTitle, current, totalTracks, failed)

					if joined && current > 0 {
						progress += "\n\n▶️ Playback has started while downloading continues."
					}

					s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
						Content: stringPtr(progress),
					})
				}
			case <-stopUpdates:
				return
			}
		}
	}()

	failedTracks := make([]string, 0)

	for idx := 0; idx < totalTracks; idx++ {
		var track interface{} = nil
		var err error

		for attempt := 0; attempt < MaxRetryAttempts; attempt++ {
			if attempt > 0 {
				time.Sleep(time.Duration(attempt) * time.Second)
				logger.InfoLogger.Printf("Retry attempt %d for track %d", attempt+1, idx+1)
			}

			track, err = c.client.DownloaderClient.DownloadPlaylistItem(url, idx, DefaultMaxDuration, DefaultMaxSize, false)
			if err == nil && track != nil {
				break
			}

			logger.ErrorLogger.Printf("Attempt %d: Failed to download playlist item %d: %v",
				attempt+1, idx+1, err)
		}

		if err != nil || track == nil {
			mu.Lock()
			failCount++
			failedTracks = append(failedTracks, fmt.Sprintf("Track #%d", idx+1))
			mu.Unlock()
			logger.ErrorLogger.Printf("All attempts failed for track %d: %v", idx+1, err)
			continue
		}

		audioTrack, ok := track.(*audio.Track)
		if !ok || audioTrack.FilePath == "" || !fileExists(audioTrack.FilePath) {
			mu.Lock()
			failCount++
			failedTracks = append(failedTracks, fmt.Sprintf("Track #%d", idx+1))
			mu.Unlock()
			logger.ErrorLogger.Printf("Invalid track data for item %d", idx+1)
			continue
		}

		audioTrack.Requester = i.Member.User.Username
		audioTrack.RequestedAt = time.Now().Unix()

		mu.Lock()
		firstTrack := !firstTrackAdded
		firstTrackAdded = true
		successCount++
		mu.Unlock()

		if firstTrack && !joinedVC {
			newChannelID, vcErr := c.client.GetUserVoiceChannel(i.GuildID, i.Member.User.ID)
			if vcErr == nil {
				channelID = newChannelID
			}

			c.client.DisableIdleMode()

			err = c.client.RobustJoinVoiceChannel(i.GuildID, channelID)
			if err != nil {
				logger.ErrorLogger.Printf("Failed to join voice channel for playlist: %v", err)
			} else {
				mu.Lock()
				joinedVC = true
				mu.Unlock()
				time.Sleep(300 * time.Millisecond)
			}
		}

		c.client.QueueManager.AddTrack(i.GuildID, audioTrack)
	}

	finalMessage := ""
	if successCount > 0 {
		finalMessage = fmt.Sprintf("✅ Added %d/%d tracks from **%s** to the queue!",
			successCount, totalTracks, playlistTitle)

		if !joinedVC {
			finalMessage += "\n\n⚠️ Couldn't join voice channel. Use /start to begin playback."
		}
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
