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
    logger.DebugLogger.Printf("Button: Parsing button ID: %s", customID)
    
    parts := strings.Split(customID, ":")
    if len(parts) != 4 || parts[0] != "search" {
        logger.ErrorLogger.Printf("Button: Invalid search button format: %s", customID)
        return nil, fmt.Errorf("invalid search button ID format")
    }
    
    trackIndex, err := strconv.Atoi(parts[1])
    if err != nil {
        logger.ErrorLogger.Printf("Button: Invalid track index in button ID: %s", parts[1])
        return nil, fmt.Errorf("invalid track index: %w", err)
    }
    
    return &SearchResultButton{
        TrackIndex: trackIndex,
        GuildID:    parts[2],
        UserID:     parts[3],
        SessionID:  fmt.Sprintf("search:%s:%s", parts[2], parts[3]),
    }, nil
}

func FormatSearchButtonID(trackIndex int, guildID, userID string) string {
    buttonID := fmt.Sprintf("search:%d:%s:%s", trackIndex, guildID, userID)
    logger.DebugLogger.Printf("Button: Formatted button ID: %s", buttonID)
    return buttonID
}

func (c *Client) HandleSearchButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
    logger.InfoLogger.Printf("Button: Handling button interaction: %s", i.MessageComponentData().CustomID)
    
    button, err := ParseSearchButton(i.MessageComponentData().CustomID)
    if err != nil {
        logger.ErrorLogger.Printf("Button: Error parsing search button ID: %v", err)
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
        logger.WarnLogger.Printf("Button: User %s tried to use another user's (%s) button", i.Member.User.ID, button.UserID)
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
    
    // First check if the cached results exist
    cacheExists := false
    var searchResults []*audio.Track
    var keys []string
    
    c.mu.RLock()
    logger.InfoLogger.Printf("Button: Looking for search results with session ID: %s", button.SessionID)
    searchResultsTemp, ok := c.searchResultsCache[button.SessionID]
    if ok {
        searchResults = make([]*audio.Track, len(searchResultsTemp))
        copy(searchResults, searchResultsTemp)
        cacheExists = true
        logger.InfoLogger.Printf("Button: Found %d results in cache for session %s", len(searchResults), button.SessionID)
    } else {
        // Log cache keys for debugging
        keys = make([]string, 0, len(c.searchResultsCache))
        for key := range c.searchResultsCache {
            keys = append(keys, key)
        }
        logger.WarnLogger.Printf("Button: No results found in cache for session %s", button.SessionID)
        logger.DebugLogger.Printf("Button: Current cache keys (%d): %v", len(keys), keys)
    }
    c.mu.RUnlock()
    
    if !cacheExists {
        // Try to salvage the situation by searching all cache entries
        logger.InfoLogger.Printf("Button: Searching all cache entries for matching results")
        c.mu.RLock()
        for key, results := range c.searchResultsCache {
            logger.DebugLogger.Printf("Button: Checking cache key: %s", key)
            if strings.Contains(key, button.GuildID) && strings.Contains(key, button.UserID) {
                logger.InfoLogger.Printf("Button: Found potential match in key: %s", key)
                searchResults = make([]*audio.Track, len(results))
                copy(searchResults, results)
                cacheExists = true
                button.SessionID = key
                break
            }
        }
        c.mu.RUnlock()
    }
    
    if !cacheExists || len(searchResults) <= button.TrackIndex {
        logger.ErrorLogger.Printf("Button: Search results expired or invalid. Cache hit: %v, Results length if hit: %d, Requested index: %d", 
            cacheExists, len(searchResults), button.TrackIndex)
        s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
            Content: stringPtr("❌ Search results expired or invalid. Please search again."),
        })
        return
    }
    
    selectedTrack := searchResults[button.TrackIndex]
    logger.InfoLogger.Printf("Button: Selected track: %s", selectedTrack.Title)
    
    channelID, err := c.GetUserVoiceChannel(button.GuildID, button.UserID)
    if err != nil {
        logger.ErrorLogger.Printf("Button: User not in a voice channel: %v", err)
        s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
            Content: stringPtr("❌ You need to be in a voice channel to play music."),
        })
        return
    }
    
    // Stop radio explicitly when a track is selected
    c.mu.Lock()
    isInIdleMode := c.isInIdleMode
    radioStreamer := c.radioStreamer
    c.isInIdleMode = false
    c.mu.Unlock()
    
    // Stop radio before processing the playlist
    if isInIdleMode && radioStreamer != nil {
        logger.InfoLogger.Println("Stopping radio before track playback")
        radioStreamer.Stop()
        time.Sleep(300 * time.Millisecond)  // Small delay to ensure clean switch
    }
    
    c.DisableIdleMode()
    
    err = c.JoinVoiceChannel(button.GuildID, channelID)
    if err != nil {
        logger.ErrorLogger.Printf("Button: Failed to join voice channel: %v", err)
        s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
            Content: stringPtr(fmt.Sprintf("❌ Failed to join voice channel: %s", err.Error())),
        })
        return
    }
    
    s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
        Content: stringPtr(fmt.Sprintf("🔍 Selected: **%s**\n⏳ Downloading...", selectedTrack.Title)),
    })
    
    logger.InfoLogger.Printf("Button: Downloading track: %s", selectedTrack.URL)
    track, err := c.udsClient.DownloadAudio(selectedTrack.URL, DefaultMaxDuration, DefaultMaxSize, false)
    if err != nil {
        logger.ErrorLogger.Printf("Button: Failed to download song: %v", err)
        s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
            Content: stringPtr(fmt.Sprintf("❌ Failed to download song: %s", err.Error())),
        })
        return
    }
    
    track.Requester = i.Member.User.Username
    track.RequestedAt = time.Now().Unix()
    
    logger.InfoLogger.Printf("Button: Adding track to queue: %s", track.Title)
    c.AddTrackToQueue(button.GuildID, track)
    
    s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
        Content: stringPtr(fmt.Sprintf("✅ Added to queue: **%s**", track.Title)),
    })
    
    logger.InfoLogger.Printf("Button: Disabling buttons on message: %s", i.Message.ID)
    emptyComponents := []discordgo.MessageComponent{}
    editMsg := &discordgo.MessageEdit{
        ID:         i.Message.ID,
        Channel:    i.ChannelID,
        Components: &emptyComponents,
    }
    _, err = s.ChannelMessageEditComplex(editMsg)
    if err != nil {
        logger.ErrorLogger.Printf("Button: Error disabling buttons: %v", err)
    }
}

func (c *Client) initSearchComponents() {
    if c.searchResultsCache == nil {
        c.searchResultsCache = make(map[string][]*audio.Track)
    }
    c.componentHandlers["search"] = c.HandleSearchButton
    logger.InfoLogger.Println("Button: Search components initialized")
}

func (c *Client) CleanupSearchCache() {
    cleanupInterval := 2 * time.Hour  // Extended to 2 hours
    ticker := time.NewTicker(cleanupInterval)
    
    logger.InfoLogger.Printf("Button: Starting search cache cleanup with interval: %v", cleanupInterval)
    
    go func() {
        for {
            select {
            case <-c.stopChan:
                ticker.Stop()
                logger.InfoLogger.Println("Button: Search cache cleanup stopped")
                return
            case <-ticker.C:
                c.mu.Lock()
                cacheSize := len(c.searchResultsCache)
                c.searchResultsCache = make(map[string][]*audio.Track)
                c.mu.Unlock()
                logger.InfoLogger.Printf("Button: Cleaned up search results cache (%d entries removed)", cacheSize)
            }
        }
    }()
}