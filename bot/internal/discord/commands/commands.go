package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/audio"
)

const (
	DefaultSearchCount    = 5
	DefaultSearchPlatform = "soundcloud"
)




type CommandFactory interface {
	Create() interface{}
}


type CommandHandler func(s *discordgo.Session, i *discordgo.InteractionCreate)


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


func FormatErrorResponse(err error) string {
	return fmt.Sprintf("❌ Error: %s", err.Error())
}


func FormatSuccessResponse(message string) string {
	return fmt.Sprintf("✅ %s", message)
}


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


func SendResponse(s *discordgo.Session, i *discordgo.InteractionCreate, message string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
		},
	})
}


func SendDeferredResponse(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
}


func EditResponse(s *discordgo.Session, i *discordgo.InteractionCreate, message string) (*discordgo.Message, error) {
	return s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(message),
	})
}


func EditResponseWithEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, message string, embeds []*discordgo.MessageEmbed) (*discordgo.Message, error) {
	return s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(message),
		Embeds:  &embeds,
	})
}


func EditResponseWithComponents(s *discordgo.Session, i *discordgo.InteractionCreate, message string, components []discordgo.MessageComponent) (*discordgo.Message, error) {
	return s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:    stringPtr(message),
		Components: &components,
	})
}


func SendEphemeralResponse(s *discordgo.Session, i *discordgo.InteractionCreate, message string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}


func DeleteResponse(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionResponseDelete(i.Interaction)
}


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
