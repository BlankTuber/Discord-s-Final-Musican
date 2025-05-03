package discord

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	DefaultMaxDuration     = 600  // 10 minutes
	DefaultMaxSize         = 50   // 50 MB
	DefaultPlaylistMax     = 10   // Default max playlist items
	DefaultPlaylistMaxOpt  = 20   // Max allowed playlist items
	DefaultSearchCount     = 5    // 5 results
	DefaultSearchPlatform  = "youtube"
)

func registerMusicCommands(registry *CommandRegistry) {
	registry.Register(&PlayCommand{})
	registry.Register(&PlaylistCommand{})
	registry.Register(&SearchCommand{})
	registry.Register(&QueueCommand{})
	registry.Register(&SkipCommand{})
	registry.Register(&ClearCommand{})
	registry.Register(&NowPlayingCommand{})
	registry.Register(&VolumeCommand{})
	registry.Register(&PopularCommand{})
	registry.Register(&RecentCommand{})
	registry.Register(&RemoveCommand{})
}

type PlayCommand struct{}

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

func (c *PlayCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	options := i.ApplicationCommandData().Options
	url := options[0].StringValue()
	
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
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("⏳ Downloading song..."),
	})
	
	// Use the updated UDS client to get track directly
	track, err := client.udsClient.DownloadAudio(url, DefaultMaxDuration, DefaultMaxSize, false)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to download song: %s", err.Error())),
		})
		return
	}
	
	track.Requester = i.Member.User.Username
	track.RequestedAt = time.Now().Unix()
	
	// Save the track to queue and start playing
	client.AddTrackToQueue(i.GuildID, track)
	
	// Update the response message
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Added to queue: **%s**", track.Title)),
	})
}

type PlaylistCommand struct{}

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

func (c *PlaylistCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	options := i.ApplicationCommandData().Options
	url := options[0].StringValue()
	
	maxItems := DefaultPlaylistMax
	if len(options) > 1 {
		maxItems = int(options[1].IntValue())
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
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("⏳ Processing playlist... This might take a while."),
	})
	
	// Use the updated UDS client to get tracks directly
	tracks, err := client.udsClient.DownloadPlaylist(url, maxItems, DefaultMaxDuration, DefaultMaxSize, false)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to download playlist: %s", err.Error())),
		})
		return
	}
	
	// Set requester for all tracks
	for _, track := range tracks {
		track.Requester = i.Member.User.Username
		track.RequestedAt = time.Now().Unix()
		
		// Add tracks to queue
		client.AddTrackToQueue(i.GuildID, track)
	}
	
	// Update the response message
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Added %d songs from playlist to the queue!", len(tracks))),
	})
}

// SearchCommand struct and interface methods remain the same
type SearchCommand struct{}

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
            Type:        discordgo.ApplicationCommandOptionInteger,
            Name:        "count",
            Description: "Number of results (1-5)",
            Required:    false,
            MinValue:    floatPtr(1),
            MaxValue:    5,
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
        limit = int(options[2].IntValue())  // Fixed: Using IntValue() for integer option
    }
    
    s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
        Content: stringPtr("🔍 Searching for: " + query + "..."),
    })
    
    // Search for tracks
    tracks, err := client.udsClient.Search(query, platform, limit, false)
    if err != nil {
        s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
            Content: stringPtr(fmt.Sprintf("❌ Search failed: %s", err.Error())),
        })
        return
    }
    
    if len(tracks) == 0 {
        s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
            Content: stringPtr(fmt.Sprintf("❌ No results found for \"%s\"", query)),
        })
        return
    }
    
    // Create a unique session ID for these results
    userID := i.Member.User.ID
    guildID := i.GuildID
    
    // Store search results in cache
    client.mu.Lock()
    sessionID := FormatSearchButtonID(0, guildID, userID)
    sessionID = sessionID[:strings.LastIndex(sessionID, ":")]
    client.searchResultsCache[sessionID] = tracks
    client.mu.Unlock()
    
    // Build the response message with buttons
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("🔍 **Search Results for \"%s\":**\n\n", query))
    
    // Create component rows with buttons for each track
    var components []discordgo.MessageComponent
    
    for i, track := range tracks {
        // Format duration
        minutes := track.Duration / 60
        seconds := track.Duration % 60
        durationStr := fmt.Sprintf("[%d:%02d]", minutes, seconds)
        
        // Add track info to message
        sb.WriteString(fmt.Sprintf("%d. **%s** %s\n", i+1, track.Title, durationStr))
        if track.ArtistName != "" {
            sb.WriteString(fmt.Sprintf("   By: %s\n", track.ArtistName))
        }
        sb.WriteString("\n")
        
        // Create a button for this track
        customID := FormatSearchButtonID(i, guildID, userID)
        button := discordgo.Button{
            Label:    fmt.Sprintf("%d. %s", i+1, track.Title),
            Style:    discordgo.PrimaryButton,
            CustomID: customID,
        }
        
        // Create action row with the button
        actionRow := discordgo.ActionsRow{
            Components: []discordgo.MessageComponent{button},
        }
        
        // Add the action row to components
        components = append(components, actionRow)
    }
    
    sb.WriteString("Click a button below to select and play a track.")
    
    // Edit the response with search results and buttons
    s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
        Content:    stringPtr(sb.String()),
        Components: &components,
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
	
	queue, currentTrack := client.GetQueueState(i.GuildID)
	
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
	
	if client.dbManager == nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Database connection not available."),
		})
		return
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
	
	sb.WriteString("\nUse `/play` with the song URL to play one of these tracks.")
	
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
	
	if client.dbManager == nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Database connection not available."),
		})
		return
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
	
	sb.WriteString("\nUse `/play` with the song URL to play one of these tracks.")
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(sb.String()),
	})
}

type RemoveCommand struct{}

func (c *RemoveCommand) Name() string {
	return "remove"
}

func (c *RemoveCommand) Description() string {
	return "Remove a song from the queue"
}

func (c *RemoveCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "position",
			Description: "Position of the song in the queue",
			Required:    true,
			MinValue:    floatPtr(1),
		},
	}
}

func (c *RemoveCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	options := i.ApplicationCommandData().Options
	position := int(options[0].IntValue()) - 1  // Convert to 0-based
	
	removed, err := client.RemoveFromQueue(i.GuildID, position)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Error removing song: %s", err.Error())),
		})
		return
	}
	
	if !removed {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ No song at position %d in the queue", position+1)),
		})
		return
	}
	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Removed song at position %d from the queue", position+1)),
	})
}