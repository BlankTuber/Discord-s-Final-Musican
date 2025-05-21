package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/audio"
)

const (
	DefaultSearchCount    = 5
	DefaultSearchPlatform = "youtube"
)

// This file defines the base command interfaces and helper functions

// CommandFactory is an interface for creating a new command instance
type CommandFactory interface {
	Create() interface{}
}

// CommandHandler is a function type for handling a command execution
type CommandHandler func(s *discordgo.Session, i *discordgo.InteractionCreate)

// CommandOption represents a slash command option
type CommandOption struct {
	Type        discordgo.ApplicationCommandOptionType
	Name        string
	Description string
	Required    bool
	Choices     []*discordgo.ApplicationCommandOptionChoice
	Options     []*discordgo.ApplicationCommandOption
	MinValue    *float64
	MaxValue    float64
}

// ToDiscordOption converts a CommandOption to a discordgo.ApplicationCommandOption
func (o *CommandOption) ToDiscordOption() *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:        o.Type,
		Name:        o.Name,
		Description: o.Description,
		Required:    o.Required,
		Choices:     o.Choices,
		Options:     o.Options,
		MinValue:    o.MinValue,
		MaxValue:    o.MaxValue,
	}
}

// FormatErrorResponse formats an error response message
func FormatErrorResponse(err error) string {
	return fmt.Sprintf("❌ Error: %s", err.Error())
}

// FormatSuccessResponse formats a success response message
func FormatSuccessResponse(message string) string {
	return fmt.Sprintf("✅ %s", message)
}

// Helper functions for command options
func stringPtr(s string) *string {
	return &s
}

func floatPtr(f float64) *float64 {
	return &f
}

func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

// SendResponse is a helper function to send a response to an interaction
func SendResponse(s *discordgo.Session, i *discordgo.InteractionCreate, message string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
		},
	})
}

// SendDeferredResponse is a helper function to send a deferred response to an interaction
func SendDeferredResponse(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
}

// EditResponse is a helper function to edit a response to an interaction
func EditResponse(s *discordgo.Session, i *discordgo.InteractionCreate, message string) (*discordgo.Message, error) {
	return s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(message),
	})
}

// EditResponseWithEmbed is a helper function to edit a response with an embed
func EditResponseWithEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, message string, embeds []*discordgo.MessageEmbed) (*discordgo.Message, error) {
	return s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(message),
		Embeds:  &embeds,
	})
}

// EditResponseWithComponents is a helper function to edit a response with components
func EditResponseWithComponents(s *discordgo.Session, i *discordgo.InteractionCreate, message string, components []discordgo.MessageComponent) (*discordgo.Message, error) {
	return s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:    stringPtr(message),
		Components: &components,
	})
}

// SendEphemeralResponse sends an ephemeral response visible only to the user
func SendEphemeralResponse(s *discordgo.Session, i *discordgo.InteractionCreate, message string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

// DeleteResponse deletes the interaction response
func DeleteResponse(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionResponseDelete(i.Interaction)
}

// FormatDuration formats a duration in seconds as MM:SS or HH:MM:SS
func FormatDuration(seconds int) string {
	if seconds < 0 {
		return "0:00"
	}

	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}

	return fmt.Sprintf("%d:%02d", minutes, secs)
}

// FormatTrackInfo formats track information in a consistent way
func FormatTrackInfo(track *audio.Track, includePosition bool) string {
	info := fmt.Sprintf("**%s** [%s]", track.Title, FormatDuration(track.Duration))

	if track.ArtistName != "" {
		info += fmt.Sprintf("\nArtist: %s", track.ArtistName)
	}

	if track.Requester != "" {
		info += fmt.Sprintf("\nRequested by: %s", track.Requester)
	}

	if includePosition && track.Position > 0 {
		info += fmt.Sprintf("\nPosition: %d", track.Position)
	}

	return info
}
