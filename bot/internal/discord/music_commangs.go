package discord

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	DefaultMaxDuration  = 600  // 10 minutes
	DefaultMaxSize      = 50   // 50 MB
	DefaultPlaylistMax  = 20   // 20 songs
	DefaultSearchCount  = 5    // 5 results
	DefaultSearchPlatform = "youtube"
)

func registerMusicCommands(registry *CommandRegistry) {
	registry.Register(&PlayCommand{})
	registry.Register(&SearchCommand{})
	registry.Register(&QueueCommand{})
	registry.Register(&SkipCommand{})
	registry.Register(&ClearCommand{})
	registry.Register(&NowPlayingCommand{})
	registry.Register(&VolumeCommand{})
	registry.Register(&PopularCommand{})
	registry.Register(&RecentCommand{})
}

type PlayCommand struct{}

func (c *PlayCommand) Name() string {
	return "play"
}

func (c *PlayCommand) Description() string {
	return "Play a song or add it to the queue"
}

func (c *PlayCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "song",
			Description: "URL or search query for the song to play",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionBoolean,
			Name:        "playlist",
			Description: "Whether to download the entire playlist (if URL is a playlist)",
			Required:    false,
		},
	}
}

func (c *PlayCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	options := i.ApplicationCommandData().Options
	query := options[0].StringValue()
	
	isPlaylist := false
	if len(options) > 1 {
		isPlaylist = options[1].BoolValue()
	}
	
	channelID, err := client.GetUserVoiceChannel(i.GuildID, i.Member.User.ID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel to use this command."),
		})
		return
	}
	
	// When someone manually plays a song, disable idle mode
	client.DisableIdleMode()
	
	err = client.JoinVoiceChannel(i.GuildID, channelID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to join voice channel: " + err.Error()),
		})
		return
	}
	
	isURL := strings.HasPrefix(query, "http://") || strings.HasPrefix(query, "https://")
	
	if isURL {
		// Process as URL
		if isPlaylist {
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("⏳ Processing playlist... This might take a while."),
			})
			
			go client.ProcessPlaylist(i.GuildID, query, i.Member.User.Username, func(message string) {
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: stringPtr(message),
				})
			})
		} else {
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("⏳ Downloading song..."),
			})
			
			go client.ProcessSong(i.GuildID, query, i.Member.User.Username, func(message string) {
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Content: stringPtr(message),
				})
			})
		}
	} else {
		// Process as search query
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("🔍 Searching for: " + query),
		})
		
		go client.ProcessSearch(i.GuildID, query, i.Member.User.Username, func(message string) {
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr(message),
			})
		})
	}
}

type SearchCommand struct{}

func (c *SearchCommand) Name() string {
	return "search"
}

func (c *SearchCommand) Description() string {
	return "Search for songs"
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
			Description: "Platform to search on (youtube, soundcloud, etc.)",
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
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "limit",
			Description: "Number of results (1-10)",
			Required:    false,
			MinValue:    floatPtr(1),
			MaxValue:    10,
		},
	}
}

func (c *SearchCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	options := i.ApplicationCommandData().Options
	query := options[0].StringValue()
	
	platform := DefaultSearchPlatform
	if len(options) > 1 {
		platform = options[1].StringValue()
	}
	
	limit := DefaultSearchCount
	if len(options) > 2 {
		limit = int(options[2].IntValue())
	}
	
	results, err := client.udsClient.Search(query, platform, limit, false)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Error searching: " + err.Error()),
		})
		return
	}
	
	if len(results) == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No results found for \"" + query + "\""),
		})
		return
	}
	
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 **Search Results for \"%s\":**\n\n", query))
	
	for i, result := range results {
		title, _ := result["title"].(string)
		uploader, _ := result["uploader"].(string)
		duration, _ := result["duration"].(float64)
		
		minutes := int(duration) / 60
		seconds := int(duration) % 60
		
		sb.WriteString(fmt.Sprintf("`%d.` **%s** [%d:%02d]\nBy: %s\n\n", i+1, title, minutes, seconds, uploader))
	}
	
	sb.WriteString("To play a result, use `/play` with the song title or number from this list.")
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(sb.String()),
	})
}

type QueueCommand struct{}

func (c *QueueCommand) Name() string {
	return "queue"
}

func (c *QueueCommand) Description() string {
	return "Show the current queue"
}

func (c *QueueCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *QueueCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	queue, currentTrack := client.GetQueueInfo(i.GuildID)
	
	if currentTrack == nil && len(queue) == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("🎵 **Queue is empty**\n\nUse `/play` to add songs."),
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
		sb.WriteString("**Queue is empty**\n\nUse `/play` to add more songs.")
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

type SkipCommand struct{}

func (c *SkipCommand) Name() string {
	return "skip"
}

func (c *SkipCommand) Description() string {
	return "Skip the current song"
}

func (c *SkipCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *SkipCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "⏭️ Skipping current song...",
		},
	})
	
	success := client.SkipSong(i.GuildID)
	if !success {
		s.InteractionResponseDelete(i.Interaction)
		s.ChannelMessageSend(i.ChannelID, "❌ No song is currently playing.")
	}
}

type ClearCommand struct{}

func (c *ClearCommand) Name() string {
	return "clear"
}

func (c *ClearCommand) Description() string {
	return "Clear the queue"
}

func (c *ClearCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *ClearCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "🧹 Queue cleared!",
		},
	})
	
	client.ClearQueue(i.GuildID)
}

type NowPlayingCommand struct{}

func (c *NowPlayingCommand) Name() string {
	return "nowplaying"
}

func (c *NowPlayingCommand) Description() string {
	return "Show the currently playing song"
}

func (c *NowPlayingCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *NowPlayingCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	currentTrack := client.GetCurrentTrack(i.GuildID)
	
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

type VolumeCommand struct{}

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

func (c *VolumeCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	options := i.ApplicationCommandData().Options
	volume := float32(options[0].FloatValue())
	
	client.SetVolume(i.GuildID, volume)
	
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🔊 Volume set to %.0f%%", volume*100),
		},
	})
}

type PopularCommand struct{}

func (c *PopularCommand) Name() string {
	return "popular"
}

func (c *PopularCommand) Description() string {
	return "Show the most popular tracks"
}

func (c *PopularCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "count",
			Description: "Number of tracks to show (1-10)",
			Required:    false,
			MinValue:    floatPtr(1),
			MaxValue:    10,
		},
	}
}

func (c *PopularCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	limit := 5
	if len(i.ApplicationCommandData().Options) > 0 {
		limit = int(i.ApplicationCommandData().Options[0].IntValue())
	}
	
	tracks, err := client.dbManager.GetPopularTracks(limit)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Error fetching popular tracks: " + err.Error()),
		})
		return
	}
	
	if len(tracks) == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("No tracks have been played yet."),
		})
		return
	}
	
	var sb strings.Builder
	sb.WriteString("🎵 **Most Popular Tracks:**\n\n")
	
	for i, track := range tracks {
		minutes := track.Duration / 60
		seconds := track.Duration % 60
		
		sb.WriteString(fmt.Sprintf("%d. **%s** [%d:%02d]\n", 
			i+1, track.Title, minutes, seconds))
		
		if track.ArtistName != "" {
			sb.WriteString(fmt.Sprintf("   Artist: %s\n", track.ArtistName))
		}
	}
	
	sb.WriteString("\nUse `/play` with the song title to play one of these tracks.")
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(sb.String()),
	})
}

type RecentCommand struct{}

func (c *RecentCommand) Name() string {
	return "recent"
}

func (c *RecentCommand) Description() string {
	return "Show recently played tracks"
}

func (c *RecentCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "count",
			Description: "Number of tracks to show (1-10)",
			Required:    false,
			MinValue:    floatPtr(1),
			MaxValue:    10,
		},
	}
}

func (c *RecentCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	limit := 5
	if len(i.ApplicationCommandData().Options) > 0 {
		limit = int(i.ApplicationCommandData().Options[0].IntValue())
	}
	
	tracks, err := client.dbManager.GetRecentTracks(limit)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Error fetching recent tracks: " + err.Error()),
		})
		return
	}
	
	if len(tracks) == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("No tracks have been played yet."),
		})
		return
	}
	
	var sb strings.Builder
	sb.WriteString("🕒 **Recently Played Tracks:**\n\n")
	
	for i, track := range tracks {
		minutes := track.Duration / 60
		seconds := track.Duration % 60
		
		sb.WriteString(fmt.Sprintf("%d. **%s** [%d:%02d]\n", 
			i+1, track.Title, minutes, seconds))
		
		if track.ArtistName != "" {
			sb.WriteString(fmt.Sprintf("   Artist: %s\n", track.ArtistName))
		}
	}
	
	sb.WriteString("\nUse `/play` with the song title to play one of these tracks.")
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(sb.String()),
	})
}