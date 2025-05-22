package components

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/discord"
	"quidque.com/discord-musican/internal/logger"
)

const (
	DefaultMaxDuration = 600
	DefaultMaxSize     = 50
)

type SearchButtonHandler struct {
	client *discord.Client
}

func NewSearchButtonHandler(client *discord.Client) *SearchButtonHandler {
	return &SearchButtonHandler{
		client: client,
	}
}

func (h *SearchButtonHandler) Prefix() string {
	return "search"
}

func (h *SearchButtonHandler) ParseButtonID(customID string) (trackIndex int, guildID, userID string, err error) {
	parts := strings.Split(customID, ":")
	if len(parts) != 4 || parts[0] != "search" {
		return 0, "", "", fmt.Errorf("invalid search button ID format")
	}

	var parseErr error
	trackIndex, parseErr = strconv.Atoi(parts[1])
	if parseErr != nil {
		return 0, "", "", fmt.Errorf("invalid track index: %w", parseErr)
	}

	return trackIndex, parts[2], parts[3], nil
}

func (h *SearchButtonHandler) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	customID := i.MessageComponentData().CustomID
	logger.InfoLogger.Printf("Handling search button interaction: %s", customID)

	trackIndex, guildID, userID, err := h.ParseButtonID(customID)
	if err != nil {
		logger.ErrorLogger.Printf("Error parsing search button ID: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Error processing button. Please try searching again."),
		})
		disableButtons(s, i.Message.ID, i.ChannelID)
		return
	}

	if i.Member.User.ID != userID {
		logger.WarnLogger.Printf("User %s tried to use another user's (%s) button", i.Member.User.ID, userID)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You can't use buttons from someone else's search results."),
		})
		return
	}

	sessionID := fmt.Sprintf("search:%s:%s", guildID, userID)

	h.client.Mu.RLock()
	var searchResults []*audio.Track
	cacheExists := false

	searchResultsTemp, ok := h.client.SearchResultsCache[sessionID]
	if ok {
		searchResults = make([]*audio.Track, len(searchResultsTemp))
		copy(searchResults, searchResultsTemp)
		cacheExists = true
		logger.InfoLogger.Printf("Found %d results in cache for session %s", len(searchResults), sessionID)
	}
	h.client.Mu.RUnlock()

	if !cacheExists {
		h.client.Mu.RLock()
		for key, results := range h.client.SearchResultsCache {
			if strings.Contains(key, guildID) && strings.Contains(key, userID) {
				logger.InfoLogger.Printf("Found potential match in key: %s", key)
				searchResults = make([]*audio.Track, len(results))
				copy(searchResults, results)
				cacheExists = true
				sessionID = key
				break
			}
		}
		h.client.Mu.RUnlock()
	}

	if !cacheExists || len(searchResults) <= trackIndex {
		logger.ErrorLogger.Printf("Search results expired or invalid. Cache hit: %v, Results length if hit: %d, Requested index: %d",
			cacheExists, len(searchResults), trackIndex)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Search results expired or invalid. Please search again."),
		})
		disableButtons(s, i.Message.ID, i.ChannelID)
		return
	}

	selectedTrack := searchResults[trackIndex]
	logger.InfoLogger.Printf("Selected track: %s", selectedTrack.Title)

	channelID, err := h.client.GetUserVoiceChannel(guildID, i.Member.User.ID)
	if err != nil {
		logger.ErrorLogger.Printf("User not in a voice channel: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel to play music."),
		})
		disableButtons(s, i.Message.ID, i.ChannelID)
		return
	}

	h.client.DisableIdleMode()

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("🔍 Selected: **%s**\n⏳ Downloading...", selectedTrack.Title)),
	})

	disableButtons(s, i.Message.ID, i.ChannelID)

	h.client.Mu.Lock()
	delete(h.client.SearchResultsCache, sessionID)
	h.client.Mu.Unlock()

	logger.InfoLogger.Printf("Downloading track: %s", selectedTrack.URL)
	track, err := h.client.DownloaderClient.DownloadAudio(selectedTrack.URL, DefaultMaxDuration, DefaultMaxSize, false)
	if err != nil {
		logger.ErrorLogger.Printf("Failed to download song: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to download song: %s", err.Error())),
		})
		return
	}

	if track == nil || track.Title == "" || track.FilePath == "" {
		logger.ErrorLogger.Printf("Downloaded track is invalid or missing file")
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to download song: The track may be too large or unavailable"),
		})
		return
	}

	track.Requester = i.Member.User.Username
	track.RequestedAt = time.Now().Unix()

	wasConnected := h.client.VoiceManager.IsConnected(guildID)
	if !wasConnected {
		err = h.client.RobustJoinVoiceChannel(guildID, channelID)
		if err != nil {
			logger.ErrorLogger.Printf("Failed to join voice channel: %v", err)

			h.client.QueueManager.AddTrack(guildID, track)

			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr(fmt.Sprintf("⚠️ Added **%s** to queue but couldn't join voice. Use /start to begin playback.", track.Title)),
			})
			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	if h.client.RadioManager.IsPlaying() {
		logger.InfoLogger.Println("Stopping radio before track playback")
		h.client.RadioManager.Stop()
		time.Sleep(300 * time.Millisecond)
	}

	logger.InfoLogger.Printf("Adding track to queue: %s", track.Title)
	h.client.QueueManager.AddTrack(guildID, track)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Added to queue: **%s**", track.Title)),
	})
}

// Helper function to disable buttons on a message
func disableButtons(s *discordgo.Session, messageID, channelID string) {
	logger.InfoLogger.Printf("Disabling buttons on message: %s", messageID)
	emptyComponents := []discordgo.MessageComponent{}
	editMsg := &discordgo.MessageEdit{
		ID:         messageID,
		Channel:    channelID,
		Components: &emptyComponents,
	}
	_, err := s.ChannelMessageEditComplex(editMsg)
	if err != nil {
		logger.ErrorLogger.Printf("Error disabling buttons: %v", err)
	}
}

func stringPtr(s string) *string {
	return &s
}
