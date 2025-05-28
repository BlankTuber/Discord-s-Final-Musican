package commands

import (
	"fmt"
	"musicbot/internal/music"
	"musicbot/internal/radio"
	"musicbot/internal/socket"
	"musicbot/internal/state"
	"musicbot/internal/voice"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type SearchCommand struct {
	voiceManager  *voice.Manager
	radioManager  *radio.Manager
	musicManager  *music.Manager
	stateManager  *state.Manager
	socketClient  *socket.Client
	searchResults map[string][]socket.SearchResult
	searchMutex   sync.RWMutex
}

func NewSearchCommand(voiceManager *voice.Manager, radioManager *radio.Manager, musicManager *music.Manager, stateManager *state.Manager, socketClient *socket.Client) *SearchCommand {
	cmd := &SearchCommand{
		voiceManager:  voiceManager,
		radioManager:  radioManager,
		musicManager:  musicManager,
		stateManager:  stateManager,
		socketClient:  socketClient,
		searchResults: make(map[string][]socket.SearchResult),
	}

	if socketClient != nil {
		socketClient.SetSearchHandler(cmd.handleSearchResults)
	}

	return cmd
}

func (c *SearchCommand) Name() string {
	return "search"
}

func (c *SearchCommand) Description() string {
	return "Search for songs to play"
}

func (c *SearchCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "query",
			Description: "Search query for songs",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "platform",
			Description: "Platform to search on",
			Required:    false,
			Choices: []*discordgo.ApplicationCommandOptionChoice{
				{Name: "SoundCloud", Value: "soundcloud"},
				{Name: "YouTube", Value: "youtube"},
				{Name: "YouTube Music", Value: "music.youtube.com"},
			},
		},
	}
}

func (c *SearchCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	options := i.ApplicationCommandData().Options
	query := options[0].StringValue()
	userID := i.Member.User.ID

	// Default to SoundCloud, but allow user to choose
	platform := "soundcloud"
	if len(options) > 1 && options[1].StringValue() != "" {
		platform = options[1].StringValue()
	}

	userVS, err := s.State.VoiceState(i.GuildID, userID)
	if err != nil || userVS == nil || userVS.ChannelID == "" {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel."),
		})
		return err
	}

	if c.socketClient == nil || !c.socketClient.IsConnected() {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Search service is not available."),
		})
		return err
	}

	platformName := "SoundCloud"
	switch platform {
	case "youtube":
		platformName = "YouTube"
	case "music.youtube.com":
		platformName = "YouTube Music"
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("🔍 Searching %s for: %s\n⏳ Please wait...", platformName, query)),
	})
	if err != nil {
		return err
	}

	// Create a unique search key that doesn't use underscores to avoid parsing issues
	searchKey := fmt.Sprintf("%s-%s", userID, i.Interaction.ID)

	err = c.socketClient.SendSearchRequest(query, platform, 5)
	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to search: %v", err)),
		})
		return err
	}

	go c.waitForSearchResults(s, i, searchKey, 2*time.Minute)

	return nil
}

func (c *SearchCommand) waitForSearchResults(s *discordgo.Session, i *discordgo.InteractionCreate, searchKey string, timeout time.Duration) {
	c.searchMutex.Lock()
	c.searchResults[searchKey] = nil
	c.searchMutex.Unlock()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		c.searchMutex.RLock()
		results := c.searchResults[searchKey]
		c.searchMutex.RUnlock()

		if results != nil {
			c.showSearchResults(s, i, results, searchKey)
			return
		}

		time.Sleep(1 * time.Second)
	}

	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr("⏱️ Search is taking longer than expected. Please try again with a different search term."),
	})
	if err != nil {
		fmt.Printf("Failed to edit interaction: %v\n", err)
	}

	c.searchMutex.Lock()
	delete(c.searchResults, searchKey)
	c.searchMutex.Unlock()
}

func (c *SearchCommand) handleSearchResults(results []socket.SearchResult) {
	c.searchMutex.Lock()
	defer c.searchMutex.Unlock()

	for key := range c.searchResults {
		c.searchResults[key] = results
	}
}

func (c *SearchCommand) showSearchResults(s *discordgo.Session, i *discordgo.InteractionCreate, results []socket.SearchResult, searchKey string) {
	if len(results) == 0 {
		_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No results found."),
		})
		if err != nil {
			fmt.Printf("Failed to edit interaction: %v\n", err)
		}
		return
	}

	content := "🎵 Search Results:\n\n"
	var components []discordgo.MessageComponent

	buttons := make([]discordgo.MessageComponent, 0)

	for idx, result := range results {
		if idx >= 5 {
			break
		}

		duration := c.formatDuration(result.Duration)
		content += fmt.Sprintf("**%d.** %s - %s (%s)\n", idx+1, result.Title, result.Uploader, duration)

		button := discordgo.Button{
			Style:    discordgo.PrimaryButton,
			Label:    strconv.Itoa(idx + 1),
			CustomID: fmt.Sprintf("search_select_%s_%d", searchKey, idx),
		}
		buttons = append(buttons, button)
	}

	actionRow := discordgo.ActionsRow{
		Components: buttons,
	}
	components = append(components, actionRow)

	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:    &content,
		Components: &components,
	})
	if err != nil {
		fmt.Printf("Failed to edit interaction: %v\n", err)
	}

	go c.cleanupSearchResults(searchKey, 5*time.Minute)
}

func (c *SearchCommand) HandleSearchSelection(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	customID := i.MessageComponentData().CustomID
	userID := i.Member.User.ID

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	// Parse the custom ID - format: "search_select_USERID-INTERACTIONID_INDEX"
	// Split by underscores, but be careful because the format is "search_select_SEARCHKEY_INDEX"
	parts := strings.Split(customID, "_")
	if len(parts) < 4 || parts[0] != "search" || parts[1] != "select" {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Invalid selection format."),
		})
		return err
	}

	// The search key is everything between "search_select_" and the last "_INDEX"
	// So we join parts[2:len(parts)-1] and the last part is the index
	searchKey := strings.Join(parts[2:len(parts)-1], "_")
	selectedIndexStr := parts[len(parts)-1]

	selectedIndex, err := strconv.Atoi(selectedIndexStr)
	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Invalid selection index."),
		})
		return err
	}

	c.searchMutex.RLock()
	results, exists := c.searchResults[searchKey]
	c.searchMutex.RUnlock()

	if !exists || results == nil || selectedIndex >= len(results) {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Search results expired."),
		})
		return err
	}

	selectedResult := results[selectedIndex]

	userVS, err := s.State.VoiceState(i.GuildID, userID)
	if err != nil || userVS == nil || userVS.ChannelID == "" {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel."),
		})
		return err
	}

	userChannelID := userVS.ChannelID
	currentChannelID := c.stateManager.GetCurrentChannel()

	if currentChannelID != "" && currentChannelID != userChannelID {
		currentBotState := c.stateManager.GetBotState()

		if currentBotState == state.StateDJ && c.musicManager.IsPlaying() {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ Bot is currently playing music in another channel."),
			})
			return err
		}

		c.radioManager.Stop()
		c.musicManager.Stop()

		err = c.voiceManager.JoinUser(i.GuildID, userID)
		if err != nil {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ Failed to join your voice channel."),
			})
			return err
		}

		time.Sleep(500 * time.Millisecond)
	} else if currentChannelID == "" {
		err = c.voiceManager.JoinUser(i.GuildID, userID)
		if err != nil {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("❌ Failed to join your voice channel."),
			})
			return err
		}
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("🎵 Downloading: %s - %s", selectedResult.Title, selectedResult.Uploader)),
	})
	if err != nil {
		return err
	}

	err = c.musicManager.RequestSong(selectedResult.URL, userID)
	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to request song: %v", err)),
		})
		return err
	}

	c.searchMutex.Lock()
	delete(c.searchResults, searchKey)
	c.searchMutex.Unlock()

	return nil
}

func (c *SearchCommand) formatDuration(seconds int) string {
	if seconds <= 0 {
		return "Unknown"
	}

	minutes := seconds / 60
	secs := seconds % 60
	return fmt.Sprintf("%d:%02d", minutes, secs)
}

func (c *SearchCommand) cleanupSearchResults(searchKey string, after time.Duration) {
	time.Sleep(after)

	c.searchMutex.Lock()
	delete(c.searchResults, searchKey)
	c.searchMutex.Unlock()
}
