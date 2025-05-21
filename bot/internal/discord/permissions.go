package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

// PermissionedCommand is an interface that commands can implement
// to specify required permissions
type PermissionedCommand interface {
	Command
	RequiredPermissions() int64 // Returns Discord permission flags
}

// CheckPermissions verifies if a user has the required permissions
func CheckPermissions(s *discordgo.Session, guildID, channelID, userID string, requiredPerms int64) (bool, error) {
	// Get the guild
	guild, err := s.State.Guild(guildID)
	if err != nil {
		guild, err = s.Guild(guildID)
		if err != nil {
			return false, fmt.Errorf("could not get guild: %w", err)
		}
	}

	// Guild owner always has all permissions
	if guild.OwnerID == userID {
		return true, nil
	}

	// Get user's permissions in the channel
	perms, err := s.State.UserChannelPermissions(userID, channelID)
	if err != nil {
		perms, err = s.UserChannelPermissions(userID, channelID)
		if err != nil {
			return false, fmt.Errorf("could not get permissions: %w", err)
		}
	}

	// Check for admin permission
	if perms&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
		return true, nil
	}

	// Check if the user has all required permissions
	return (perms & requiredPerms) == requiredPerms, nil
}

// Add to permissions.go
func CheckDJRole(s *discordgo.Session, guildID, userID string) (bool, error) {
	// Get member
	member, err := s.State.Member(guildID, userID)
	if err != nil {
		member, err = s.GuildMember(guildID, userID)
		if err != nil {
			return false, fmt.Errorf("could not get member: %w", err)
		}
	}

	// Get guild roles
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		return false, fmt.Errorf("could not get guild roles: %w", err)
	}

	// Look for "DJ" role and check if member has it
	for _, role := range roles {
		if role.Name == "DJ" {
			for _, memberRole := range member.Roles {
				if memberRole == role.ID {
					return true, nil
				}
			}
		}
	}

	return false, nil
}
