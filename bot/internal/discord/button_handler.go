package discord

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/logger"
)

// SearchResultButton represents a button for search results
type SearchResultButton struct {
	TrackIndex int
	GuildID    string
	UserID     string
	SessionID  string
}

// ParseSearchButton parses a button custom ID into a SearchResultButton
func ParseSearchButton(customID string) (*SearchResultButton, error) {
	parts := strings.Split(customID, ":")
	if len(parts) != 4 || parts[0] != "search" {
		return nil, fmt.Errorf("invalid search button ID format")
	}
	
	trackIndex, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid track index: %w", err)
	}
	
	return &SearchResultButton{
		TrackIndex: trackIndex,
		GuildID:    parts[2],
		UserID:     parts[3],
		SessionID:  customID,
	}, nil
}

// FormatSearchButtonID formats a search button ID
func FormatSearchButtonID(trackIndex int, guildID, userID string) string {
	return fmt.Sprintf("search:%d:%s:%s", trackIndex, guildID, userID)
}

// HandleSearchButton handles a button click on search results
func (c *Client) HandleSearchButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get the button data
	button, err := ParseSearchButton(i.MessageComponentData().CustomID)
	if err != nil {
		logger.ErrorLogger.Printf("Error parsing search button ID: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Error processing button. Please try searching again.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	
	// Verify that the user who clicked is the one who searched
	if i.Member.User.ID != button.UserID {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ You can't use buttons from someone else's search results.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	
	// First, acknowledge the interaction to avoid timeout
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	
	// Get the cached search results
	c.mu.RLock()
	searchResults, ok := c.searchResultsCache[button.SessionID]
	c.mu.RUnlock()
	
	if !ok || len(searchResults) <= button.TrackIndex {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Search results expired or invalid. Please search again."),
		})
		return
	}
	
	// Get the selected track
	selectedTrack := searchResults[button.TrackIndex]
	
	// Get the user's voice channel
	channelID, err := c.GetUserVoiceChannel(button.GuildID, button.UserID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel to play music."),
		})
		return
	}
	
	// Disable idle mode
	c.DisableIdleMode()
	
	// Join voice channel
	err = c.JoinVoiceChannel(button.GuildID, channelID)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to join voice channel: %s", err.Error())),
		})
		return
	}
	
	// Update the message to show downloading
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("🔍 Selected: **%s**\n⏳ Downloading...", selectedTrack.Title)),
	})
	
	// Download the track
	track, err := c.udsClient.DownloadAudio(selectedTrack.URL, DefaultMaxDuration, DefaultMaxSize, false)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to download song: %s", err.Error())),
		})
		return
	}
	
	track.Requester = i.Member.User.Username
	track.RequestedAt = time.Now().Unix()
	
	// Add to queue and play
	c.AddTrackToQueue(button.GuildID, track)
	
	// Update the response message
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Added to queue: **%s**", track.Title)),
	})
	
	// Disable the buttons on the original message
	s.MessageEdit(i.ChannelID, i.Message.ID, &discordgo.MessageEdit{
		Components: []discordgo.MessageComponent{}, // Remove all components
	})
}

// Initialization function to add search results cache
func (c *Client) initSearchComponents() {
	// Initialize search results cache if it doesn't exist
	if c.searchResultsCache == nil {
		c.searchResultsCache = make(map[string][]*audio.Track)
	}
	
	// Register component handler for search buttons
	c.componentHandlers["search"] = c.HandleSearchButton
}

// CleanupSearchCache cleans up expired search results
func (c *Client) CleanupSearchCache() {
	cleanupInterval := 5 * time.Minute
	ticker := time.NewTicker(cleanupInterval)
	
	go func() {
		for {
			select {
			case <-c.stopChan:
				ticker.Stop()
				return
			case <-ticker.C:
				c.mu.Lock()
				// Just reset the cache every interval for simplicity
				// In production, you might want to track timestamps and remove only expired entries
				c.searchResultsCache = make(map[string][]*audio.Track)
				c.mu.Unlock()
				logger.DebugLogger.Println("Cleaned up search results cache")
			}
		}
	}()
}