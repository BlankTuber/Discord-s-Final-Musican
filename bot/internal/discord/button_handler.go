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

type SearchResultButton struct {
    TrackIndex int
    GuildID    string
    UserID     string
    SessionID  string
}

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

func FormatSearchButtonID(trackIndex int, guildID, userID string) string {
    return fmt.Sprintf("search:%d:%s:%s", trackIndex, guildID, userID)
}

func (c *Client) HandleSearchButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
    
    s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
    })
    
    c.mu.RLock()
    searchResults, ok := c.searchResultsCache[button.SessionID]
    c.mu.RUnlock()
    
    if !ok || len(searchResults) <= button.TrackIndex {
        s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
            Content: stringPtr("❌ Search results expired or invalid. Please search again."),
        })
        return
    }
    
    selectedTrack := searchResults[button.TrackIndex]
    
    channelID, err := c.GetUserVoiceChannel(button.GuildID, button.UserID)
    if err != nil {
        s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
            Content: stringPtr("❌ You need to be in a voice channel to play music."),
        })
        return
    }
    
    c.DisableIdleMode()
    
    err = c.JoinVoiceChannel(button.GuildID, channelID)
    if err != nil {
        s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
            Content: stringPtr(fmt.Sprintf("❌ Failed to join voice channel: %s", err.Error())),
        })
        return
    }
    
    s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
        Content: stringPtr(fmt.Sprintf("🔍 Selected: **%s**\n⏳ Downloading...", selectedTrack.Title)),
    })
    
    track, err := c.udsClient.DownloadAudio(selectedTrack.URL, DefaultMaxDuration, DefaultMaxSize, false)
    if err != nil {
        s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
            Content: stringPtr(fmt.Sprintf("❌ Failed to download song: %s", err.Error())),
        })
        return
    }
    
    track.Requester = i.Member.User.Username
    track.RequestedAt = time.Now().Unix()
    
    c.AddTrackToQueue(button.GuildID, track)
    
    s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
        Content: stringPtr(fmt.Sprintf("✅ Added to queue: **%s**", track.Title)),
    })
    
    emptyComponents := []discordgo.MessageComponent{}
    editMsg := &discordgo.MessageEdit{
        ID:         i.Message.ID,
        Channel:    i.ChannelID,
        Components: &emptyComponents,
    }
    _, err = s.ChannelMessageEditComplex(editMsg)
    if err != nil {
        logger.ErrorLogger.Printf("Error disabling buttons: %v", err)
    }
}

func (c *Client) initSearchComponents() {
    if c.searchResultsCache == nil {
        c.searchResultsCache = make(map[string][]*audio.Track)
    }
    c.componentHandlers["search"] = c.HandleSearchButton
}

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
                c.searchResultsCache = make(map[string][]*audio.Track)
                c.mu.Unlock()
                logger.DebugLogger.Println("Cleaned up search results cache")
            }
        }
    }()
}