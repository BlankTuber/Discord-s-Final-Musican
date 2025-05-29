package commands

import (
	"musicbot/internal/permissions"

	"github.com/bwmarrin/discordgo"
)

type PermissionWrapper struct {
	command           Command
	requiredLevel     permissions.Level
	permissionManager *permissions.Manager
}

func NewPermissionWrapper(command Command, requiredLevel permissions.Level, permissionManager *permissions.Manager) *PermissionWrapper {
	return &PermissionWrapper{
		command:           command,
		requiredLevel:     requiredLevel,
		permissionManager: permissionManager,
	}
}

func (w *PermissionWrapper) Name() string {
	return w.command.Name()
}

func (w *PermissionWrapper) Description() string {
	return w.command.Description()
}

func (w *PermissionWrapper) Options() []*discordgo.ApplicationCommandOption {
	return w.command.Options()
}

func (w *PermissionWrapper) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	if w.requiredLevel != permissions.LevelUser {
		hasPermission, err := w.permissionManager.HasPermission(s, i.GuildID, i.Member.User.ID, w.requiredLevel)
		if err != nil {
			return err
		}

		if !hasPermission {
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "❌ You need " + w.requiredLevel.String() + " permissions to use this command.",
				},
			})
			return err
		}
	}

	return w.command.Execute(s, i)
}
