package commands

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/discord"
	"quidque.com/discord-musican/internal/logger"
)

// PlayCommand handles the /play command
type PlayCommand struct {
	client *discord.Client
}

func NewPlayCommand(client *discord.Client) *PlayCommand {
	return &PlayCommand{
		client: client,
	}
}

func (c *PlayCommand) Name() string {
	return "play"
}

func (c *PlayCommand) Description() string {
	return "Play a song by URL"
}

func (c *PlayCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "url",
			Description: "URL of the song to play",
			Required:    true,
		},
	}
}

func (c *PlayCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	url := options[0].StringValue()

	// Verify user is in a voice channel first (but don't join yet)
	channelID, err := c.client.GetUserVoiceChannel(i.GuildID, i.Member.User.ID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel to use this command."),
		})
		return
	}

	// Disable idle mode to prevent automatic VC changes during download
	c.client.DisableIdleMode()

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("⏳ Downloading song..."),
	})

	// Download the track first
	track, err := c.client.DownloaderClient.DownloadAudio(url, DefaultMaxDuration, DefaultMaxSize, false)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to download song: %s", err.Error())),
		})
		return
	}

	track.Requester = i.Member.User.Username
	track.RequestedAt = time.Now().Unix()

	// Add to queue first (this will play the track when we join)
	c.client.QueueManager.AddTrack(i.GuildID, track)

	// Only join VC after download is complete and track is in queue
	err = c.client.RobustJoinVoiceChannel(i.GuildID, channelID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("⚠️ Added track to queue but had trouble joining voice channel: %s\nTry using /start if playback doesn't begin.", err.Error())),
		})
		return
	}

	// Double check connection and playback
	if !c.client.VoiceManager.IsConnected(i.GuildID) {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("⚠️ Added to queue, but voice connection was lost. Try /start to begin playback."),
		})
		return
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Added to queue and playback started: **%s**", track.Title)),
	})
}

// QueueCommand handles the /queue command
type QueueCommand struct {
	client *discord.Client
}

func NewQueueCommand(client *discord.Client) *QueueCommand {
	return &QueueCommand{
		client: client,
	}
}

func (c *QueueCommand) Name() string {
	return "queue"
}

func (c *QueueCommand) Description() string {
	return "Show the current queue"
}

func (c *QueueCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *QueueCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	queue := c.client.QueueManager.GetQueue(i.GuildID)
	currentTrack := c.client.QueueManager.GetCurrentTrack(i.GuildID)

	if currentTrack == nil && len(queue) == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("🎵 **Queue is empty**\n\nUse `/play`, `/playlist`, or `/search` to add songs."),
		})
		return
	}

	var sb strings.Builder
	sb.WriteString("🎵 **Current Queue:**\n\n")

	if currentTrack != nil {
		minutes := currentTrack.Duration / 60
		seconds := currentTrack.Duration % 60

		sb.WriteString(fmt.Sprintf("**Now Playing:** %s [%d:%02d]\n",
			currentTrack.Title, minutes, seconds))
		sb.WriteString(fmt.Sprintf("Requested by: %s\n\n", currentTrack.Requester))
	}

	if len(queue) == 0 {
		sb.WriteString("**Queue is empty**\n\nUse `/play`, `/playlist`, or `/search` to add more songs.")
	} else {
		sb.WriteString("**Up Next:**\n")

		totalDuration := 0
		for i, track := range queue {
			if i >= 10 {
				sb.WriteString(fmt.Sprintf("\n... and %d more songs", len(queue)-10))
				break
			}

			minutes := track.Duration / 60
			seconds := track.Duration % 60

			sb.WriteString(fmt.Sprintf("%d. **%s** [%d:%02d] - %s\n",
				i+1, track.Title, minutes, seconds, track.Requester))

			totalDuration += track.Duration
		}

		totalMinutes := totalDuration / 60
		totalHours := totalMinutes / 60
		remainingMinutes := totalMinutes % 60

		sb.WriteString(fmt.Sprintf("\n**%d songs in queue** | Total Duration: %d:%02d hours",
			len(queue), totalHours, remainingMinutes))
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(sb.String()),
	})
}

// SkipCommand handles the /skip command
type SkipCommand struct {
	client *discord.Client
}

func NewSkipCommand(client *discord.Client) *SkipCommand {
	return &SkipCommand{
		client: client,
	}
}

func (c *SkipCommand) Name() string {
	return "skip"
}

func (c *SkipCommand) Description() string {
	return "Skip the current song"
}

func (c *SkipCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *SkipCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	isDj, _ := discord.CheckDJRole(s, i.GuildID, i.Member.User.ID)

	if !isDj {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You need to be a DJ to skip songs.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "⏭️ Skipping current song...",
		},
	})

	success := c.client.VoiceManager.SkipTrack(i.GuildID)
	if !success {
		s.InteractionResponseDelete(i.Interaction)
		s.ChannelMessageSend(i.ChannelID, "❌ No song is currently playing.")
	}
}

// NowPlayingCommand handles the /nowplaying command
type NowPlayingCommand struct {
	client *discord.Client
}

func NewNowPlayingCommand(client *discord.Client) *NowPlayingCommand {
	return &NowPlayingCommand{
		client: client,
	}
}

func (c *NowPlayingCommand) Name() string {
	return "nowplaying"
}

func (c *NowPlayingCommand) Description() string {
	return "Show the currently playing song"
}

func (c *NowPlayingCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *NowPlayingCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	// Check if radio is playing
	if c.client.RadioManager.IsPlaying() {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("📻 **Now Playing: Radio Stream**\n🔊 Volume: %.0f%%\n🔗 Stream URL: %s",
				c.client.RadioManager.GetVolume()*100, c.client.RadioManager.GetURL())),
		})
		return
	}

	currentTrack := c.client.QueueManager.GetCurrentTrack(i.GuildID)

	if currentTrack == nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No song is currently playing."),
		})
		return
	}

	minutes := currentTrack.Duration / 60
	seconds := currentTrack.Duration % 60

	content := fmt.Sprintf("🎵 **Now Playing:** %s [%d:%02d]\n",
		currentTrack.Title, minutes, seconds)

	if currentTrack.URL != "" {
		content += fmt.Sprintf("🔗 **Link:** %s\n", currentTrack.URL)
	}

	if currentTrack.ArtistName != "" {
		content += fmt.Sprintf("👤 **Artist:** %s\n", currentTrack.ArtistName)
	}

	content += fmt.Sprintf("🎧 **Requested by:** %s\n", currentTrack.Requester)

	if currentTrack.ThumbnailURL != "" {
		embeds := []*discordgo.MessageEmbed{
			{
				Title: "Now Playing",
				Image: &discordgo.MessageEmbedImage{
					URL: currentTrack.ThumbnailURL,
				},
			},
		}

		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(content),
			Embeds:  &embeds,
		})
	} else {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(content),
		})
	}
}

// VolumeCommand handles the /volume command
type VolumeCommand struct {
	client *discord.Client
}

func NewVolumeCommand(client *discord.Client) *VolumeCommand {
	return &VolumeCommand{
		client: client,
	}
}

func (c *VolumeCommand) Name() string {
	return "volume"
}

func (c *VolumeCommand) Description() string {
	return "Set the playback volume (0.0 to 1.0)"
}

func (c *VolumeCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionNumber,
			Name:        "level",
			Description: "Volume level (0.0 to 1.0)",
			Required:    true,
			MinValue:    floatPtr(0.0),
			MaxValue:    1.0,
		},
	}
}

func (c *VolumeCommand) RequiredPermissions() int64 {
	return discordgo.PermissionManageServer
}

func (c *VolumeCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	volume := float32(options[0].FloatValue())

	c.client.VoiceManager.SetVolume(volume)
	c.client.RadioManager.SetVolume(volume)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🔊 Volume set to %.0f%%", volume*100),
		},
	})
}

// StartCommand handles the /start command
type StartCommand struct {
	client *discord.Client
}

func NewStartCommand(client *discord.Client) *StartCommand {
	return &StartCommand{
		client: client,
	}
}

func (c *StartCommand) Name() string {
	return "start"
}

func (c *StartCommand) Description() string {
	return "Start playback from the queue"
}

func (c *StartCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

// StartCommand handles the /start command
func (c *StartCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	// Aggressively disable idle mode first thing
	c.client.DisableIdleMode()

	// Check if user is in a voice channel
	channelID, err := c.client.GetUserVoiceChannel(i.GuildID, i.Member.User.ID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel to use this command."),
		})
		return
	}

	// Stop radio if it's playing
	if c.client.RadioManager.IsPlaying() {
		c.client.RadioManager.Stop()
		time.Sleep(500 * time.Millisecond)
	}

	// Check if we have a paused player first
	playerState := c.client.VoiceManager.GetPlayerState(i.GuildID)
	if playerState == audio.StatePaused {
		// Make sure we're in the same channel as the user before resuming
		if !c.client.VoiceManager.IsConnectedToChannel(i.GuildID, channelID) {
			// Move to user's channel first
			err = c.client.RobustJoinVoiceChannel(i.GuildID, channelID)
			if err != nil {
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: stringPtr(fmt.Sprintf("❌ Failed to join your voice channel: %v", err)),
				})
				return
			}
		}

		logger.InfoLogger.Printf("Found paused player, attempting to resume")
		success := c.client.VoiceManager.ResumePlayback(i.GuildID)
		if success {
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("▶️ Playback resumed!"),
			})
			return
		}
	}

	// Check for current track and queue
	currentTrack := c.client.QueueManager.GetCurrentTrack(i.GuildID)
	queue := c.client.QueueManager.GetQueue(i.GuildID)

	// Get player directly to check for current track
	var playerCurrentTrack *audio.Track
	player := c.client.VoiceManager.GetPlayer(i.GuildID)
	if player != nil {
		playerCurrentTrack = player.GetCurrentTrack()
	}

	// Check if we have any tracks to play
	if currentTrack == nil && playerCurrentTrack == nil && len(queue) == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Queue is empty. Nothing to start."),
		})
		return
	}

	// Join the user's voice channel
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("🔄 Joining voice channel and preparing playback..."),
	})

	err = c.client.JoinVoiceChannel(i.GuildID, channelID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to join voice channel: %v", err)),
		})
		return
	}

	// Create a new playlist using both current track and queue
	var tracksToPlay []*audio.Track

	// First check if we have a valid current track from queue manager
	if currentTrack != nil && currentTrack.FilePath != "" {
		if _, err := os.Stat(currentTrack.FilePath); err == nil {
			tracksToPlay = append(tracksToPlay, currentTrack)
		}
	}

	// Also check player's current track if different from queue manager's
	if playerCurrentTrack != nil && playerCurrentTrack.FilePath != "" {
		// Check if this track is already added (from currentTrack)
		isDuplicate := false
		if currentTrack != nil && currentTrack.FilePath == playerCurrentTrack.FilePath {
			isDuplicate = true
		}

		if !isDuplicate {
			if _, err := os.Stat(playerCurrentTrack.FilePath); err == nil {
				tracksToPlay = append(tracksToPlay, playerCurrentTrack)
			}
		}
	}

	// Add queue items if any
	for _, track := range queue {
		if track.FilePath != "" {
			if _, err := os.Stat(track.FilePath); err == nil {
				tracksToPlay = append(tracksToPlay, track)
			}
		}
	}

	if len(tracksToPlay) == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No valid tracks found. Files may be missing."),
		})
		return
	}

	// Update status to show we're restarting playback
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("⏹️ Stopping current playback and preparing tracks..."),
	})

	c.client.VoiceManager.StopAllPlayback()
	time.Sleep(800 * time.Millisecond)

	// Clear existing queue and rebuild with our tracks
	c.client.QueueManager.ClearQueue(i.GuildID)
	time.Sleep(300 * time.Millisecond)

	// Add tracks back to queue
	addedCount := c.client.QueueManager.AddTracks(i.GuildID, tracksToPlay)

	if addedCount == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to add tracks to queue. Please try again."),
		})
		return
	}

	// Start playback
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("▶️ Starting playback..."),
	})

	c.client.StartPlayback(i.GuildID)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Playback started with %d track(s)!", addedCount)),
	})
}

// PauseCommand handles the /pause command
type PauseCommand struct {
	client *discord.Client
}

func NewPauseCommand(client *discord.Client) *PauseCommand {
	return &PauseCommand{
		client: client,
	}
}

func (c *PauseCommand) Name() string {
	return "pause"
}

func (c *PauseCommand) Description() string {
	return "Pause the currently playing song"
}

func (c *PauseCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *PauseCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "⏸️ Pausing playback...",
		},
	})

	// Check if radio is playing and pause it if so
	if c.client.RadioManager.IsPlaying() {
		c.client.RadioManager.Pause()
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("⏸️ Radio stream paused. Use `/start` to resume."),
		})
		return
	}

	success := c.client.VoiceManager.PausePlayback(i.GuildID)

	if !success {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No song is currently playing."),
		})
		return
	}

	// Store the current track in QueueManager to make it accessible by other commands
	currentPlayer := c.client.VoiceManager.GetPlayer(i.GuildID)
	if currentPlayer != nil {
		pausedTrack := currentPlayer.GetCurrentTrack()
		if pausedTrack != nil {
			c.client.QueueManager.SetCurrentTrack(i.GuildID, pausedTrack)
		}
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("⏸️ Playback paused. Use `/start` to resume."),
	})
}

// SearchCommand handles the /search command
type SearchCommand struct {
	client *discord.Client
}

func NewSearchCommand(client *discord.Client) *SearchCommand {
	return &SearchCommand{
		client: client,
	}
}

func (c *SearchCommand) Name() string {
	return "search"
}

func (c *SearchCommand) Description() string {
	return "Search and display results with buttons"
}

func (c *SearchCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "query",
			Description: "Search query",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "platform",
			Description: "Platform to search on",
			Required:    false,
			Choices: []*discordgo.ApplicationCommandOptionChoice{
				{
					Name:  "YouTube",
					Value: "youtube",
				},
				{
					Name:  "SoundCloud",
					Value: "soundcloud",
				},
				{
					Name:  "YouTube Music",
					Value: "ytmusic",
				},
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "count",
			Description: "Number of results (1-5) - Enter as text",
			Required:    false,
		},
	}
}

func (c *SearchCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Use ephemeral response (only visible to the command user)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})

	options := i.ApplicationCommandData().Options
	query := options[0].StringValue()

	platform := DefaultSearchPlatform
	for _, opt := range options[1:] {
		if opt.Name == "platform" {
			platform = opt.StringValue()
			break
		}
	}

	limit := DefaultSearchCount
	for _, opt := range options[1:] {
		if opt.Name == "count" {
			countStr := opt.StringValue()
			if countStr != "" {
				parsedCount, err := strconv.Atoi(countStr)
				if err != nil {
					s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
						Content: stringPtr(fmt.Sprintf("⚠️ Invalid number provided for count ('%s'). Using default count of %d.", countStr, DefaultSearchCount)),
					})
					limit = DefaultSearchCount
				} else {
					if parsedCount >= 1 && parsedCount <= 5 {
						limit = parsedCount
					} else {
						s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
							Content: stringPtr(fmt.Sprintf("⚠️ Count must be between 1 and 5. Using default count of %d.", DefaultSearchCount)),
						})
						limit = DefaultSearchCount
					}
				}
			}
			break
		}
	}

	logger.InfoLogger.Printf("Searching for '%s' on %s with limit %d", query, platform, limit)
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("🔍 Searching for: " + query + "..."),
	})
	if err != nil {
		logger.ErrorLogger.Printf("Error updating search status: %v", err)
	}

	tracks, err := c.client.DownloaderClient.Search(query, platform, limit, false)
	if err != nil {
		logger.ErrorLogger.Printf("Search failed: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Search failed: %s", err.Error())),
		})
		return
	}

	if len(tracks) == 0 {
		logger.InfoLogger.Printf("No search results found for query: %s", query)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ No results found for \"%s\"", query)),
		})
		return
	}

	logger.InfoLogger.Printf("Found %d search results for '%s'", len(tracks), query)

	userID := i.Member.User.ID
	guildID := i.GuildID

	sessionID := fmt.Sprintf("search:%s:%s", guildID, userID)

	// Store the search results in the cache
	c.client.Mu.Lock()
	c.client.SearchResultsCache[sessionID] = tracks
	c.client.Mu.Unlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 **Search Results for \"%s\":**\n\n", query))

	var components []discordgo.MessageComponent

	for idx, track := range tracks {
		minutes := track.Duration / 60
		seconds := track.Duration % 60
		durationStr := fmt.Sprintf("[%d:%02d]", minutes, seconds)

		sb.WriteString(fmt.Sprintf("%d. **%s** %s\n", idx+1, track.Title, durationStr))
		if track.ArtistName != "" {
			sb.WriteString(fmt.Sprintf("   By: %s\n", track.ArtistName))
		}
		sb.WriteString("\n")

		customID := fmt.Sprintf("search:%d:%s:%s", idx, guildID, userID)

		buttonLabel := fmt.Sprintf("%d. %s", idx+1, track.Title)
		if len(buttonLabel) > 80 {
			buttonLabel = buttonLabel[:77] + "..."
		}

		button := discordgo.Button{
			Label:    buttonLabel,
			Style:    discordgo.PrimaryButton,
			CustomID: customID,
		}

		actionRow := discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{button},
		}

		components = append(components, actionRow)
	}

	sb.WriteString("Click a button below to select and play a track.")

	messageContent := sb.String()

	// Try to edit the message with content and components
	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:    stringPtr(messageContent),
		Components: &components,
	})

	if err != nil {
		logger.ErrorLogger.Printf("Error updating search results: %v", err)

		// Fallback approach: just send the content without components
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(messageContent + "\n\n❌ Error displaying buttons. Please try again or use /play command directly."),
		})

		if err != nil {
			logger.ErrorLogger.Printf("Error with fallback message too: %v", err)
		}
	} else {
		logger.InfoLogger.Printf("Successfully updated search results with buttons")
	}
}
