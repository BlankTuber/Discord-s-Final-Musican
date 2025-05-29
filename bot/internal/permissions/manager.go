package permissions

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type Manager struct {
	config Config
}

func NewManager(config Config) *Manager {
	return &Manager{
		config: config,
	}
}

func (m *Manager) HasPermission(session *discordgo.Session, guildID, userID string, requiredLevel Level) (bool, error) {
	if requiredLevel == LevelUser {
		return true, nil
	}

	member, err := session.GuildMember(guildID, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get guild member: %w", err)
	}

	guild, err := session.Guild(guildID)
	if err != nil {
		return false, fmt.Errorf("failed to get guild: %w", err)
	}

	userRoles := make(map[string]bool)
	for _, roleID := range member.Roles {
		for _, guildRole := range guild.Roles {
			if guildRole.ID == roleID {
				userRoles[strings.ToLower(guildRole.Name)] = true
				break
			}
		}
	}

	switch requiredLevel {
	case LevelDJ:
		return m.hasDJPermission(userRoles), nil
	case LevelAdmin:
		return m.hasAdminPermission(userRoles), nil
	default:
		return false, fmt.Errorf("unknown permission level: %v", requiredLevel)
	}
}

func (m *Manager) hasDJPermission(userRoles map[string]bool) bool {
	if m.config.DJRoleName != "" && userRoles[strings.ToLower(m.config.DJRoleName)] {
		return true
	}
	return m.hasAdminPermission(userRoles)
}

func (m *Manager) hasAdminPermission(userRoles map[string]bool) bool {
	if m.config.AdminRoleName != "" && userRoles[strings.ToLower(m.config.AdminRoleName)] {
		return true
	}
	return userRoles["administrator"] || userRoles["admin"]
}

func (m *Manager) CheckPermissionWithResponse(session *discordgo.Session, interaction *discordgo.InteractionCreate, requiredLevel Level) (bool, error) {
	hasPermission, err := m.HasPermission(session, interaction.GuildID, interaction.Member.User.ID, requiredLevel)
	if err != nil {
		return false, err
	}

	if !hasPermission {
		response := fmt.Sprintf("❌ You need %s permissions to use this command.", requiredLevel.String())
		_, err = session.InteractionResponseEdit(interaction.Interaction, &discordgo.WebhookEdit{
			Content: &response,
		})
		return false, err
	}

	return true, nil
}

func (m *Manager) GetRequiredRoleName(level Level) string {
	switch level {
	case LevelDJ:
		if m.config.DJRoleName != "" {
			return m.config.DJRoleName
		}
		return "DJ"
	case LevelAdmin:
		if m.config.AdminRoleName != "" {
			return m.config.AdminRoleName
		}
		return "Admin"
	default:
		return "User"
	}
}
