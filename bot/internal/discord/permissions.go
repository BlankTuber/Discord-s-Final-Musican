package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)



type PermissionedCommand interface {
	Command
	RequiredPermissions() int64 
}


func CheckPermissions(s *discordgo.Session, guildID, channelID, userID string, requiredPerms int64) (bool, error) {
	
	guild, err := s.State.Guild(guildID)
	if err != nil {
		guild, err = s.Guild(guildID)
		if err != nil {
			return false, fmt.Errorf("could not get guild: %w", err)
		}
	}

	
	if guild.OwnerID == userID {
		return true, nil
	}

	
	perms, err := s.State.UserChannelPermissions(userID, channelID)
	if err != nil {
		perms, err = s.UserChannelPermissions(userID, channelID)
		if err != nil {
			return false, fmt.Errorf("could not get permissions: %w", err)
		}
	}

	
	if perms&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
		return true, nil
	}

	
	return (perms & requiredPerms) == requiredPerms, nil
}


func CheckDJRole(s *discordgo.Session, guildID, userID string) (bool, error) {
	
	member, err := s.State.Member(guildID, userID)
	if err != nil {
		member, err = s.GuildMember(guildID, userID)
		if err != nil {
			return false, fmt.Errorf("could not get member: %w", err)
		}
	}

	
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		return false, fmt.Errorf("could not get guild roles: %w", err)
	}

	
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
