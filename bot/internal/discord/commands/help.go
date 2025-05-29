package commands

import (
	"fmt"
	"musicbot/internal/permissions"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type HelpCommand struct {
	permissionManager *permissions.Manager
	commandRegistry   map[string]HelpCommandInfo
}

type HelpCommandInfo struct {
	Description   string
	RequiredLevel permissions.Level
	Category      string
}

func NewHelpCommand(permissionManager *permissions.Manager) *HelpCommand {
	cmd := &HelpCommand{
		permissionManager: permissionManager,
		commandRegistry:   make(map[string]HelpCommandInfo),
	}

	cmd.registerCommands()
	return cmd
}

func (c *HelpCommand) registerCommands() {
	c.commandRegistry = map[string]HelpCommandInfo{
		"help": {
			Description:   "Show this help message",
			RequiredLevel: permissions.LevelUser,
			Category:      "Utility",
		},
		"ping": {
			Description:   "Check bot latency and response time",
			RequiredLevel: permissions.LevelUser,
			Category:      "Utility",
		},
		"join": {
			Description:   "Join your voice channel",
			RequiredLevel: permissions.LevelUser,
			Category:      "Voice",
		},
		"leave": {
			Description:   "Leave current voice channel and return to idle",
			RequiredLevel: permissions.LevelUser,
			Category:      "Voice",
		},
		"play": {
			Description:   "Play a song from URL",
			RequiredLevel: permissions.LevelUser,
			Category:      "Music",
		},
		"playlist": {
			Description:   "Play a playlist from URL",
			RequiredLevel: permissions.LevelDJ,
			Category:      "Music",
		},
		"search": {
			Description:   "Search for songs to play",
			RequiredLevel: permissions.LevelUser,
			Category:      "Music",
		},
		"queue": {
			Description:   "Show the current music queue",
			RequiredLevel: permissions.LevelUser,
			Category:      "Music",
		},
		"nowplaying": {
			Description:   "Show what's currently playing",
			RequiredLevel: permissions.LevelUser,
			Category:      "Music",
		},
		"skip": {
			Description:   "Skip the current song",
			RequiredLevel: permissions.LevelUser,
			Category:      "Music",
		},
		"pause": {
			Description:   "Pause music and switch to idle mode",
			RequiredLevel: permissions.LevelUser,
			Category:      "Music",
		},
		"resume": {
			Description:   "Resume paused music",
			RequiredLevel: permissions.LevelUser,
			Category:      "Music",
		},
		"clear": {
			Description:   "Clear the music queue",
			RequiredLevel: permissions.LevelDJ,
			Category:      "Music",
		},
		"changestream": {
			Description:   "Change the radio stream",
			RequiredLevel: permissions.LevelDJ,
			Category:      "Radio",
		},
	}
}

func (c *HelpCommand) Name() string {
	return "help"
}

func (c *HelpCommand) Description() string {
	return "Show available commands and their descriptions"
}

func (c *HelpCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "category",
			Description: "Show commands from a specific category",
			Required:    false,
			Choices: []*discordgo.ApplicationCommandOptionChoice{
				{Name: "All", Value: "all"},
				{Name: "Utility", Value: "utility"},
				{Name: "Voice", Value: "voice"},
				{Name: "Music", Value: "music"},
				{Name: "Radio", Value: "radio"},
			},
		},
	}
}

func (c *HelpCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: c.generateHelpMessage(s, i),
		},
	})
	return err
}

func (c *HelpCommand) generateHelpMessage(s *discordgo.Session, i *discordgo.InteractionCreate) string {
	options := i.ApplicationCommandData().Options
	selectedCategory := "all"
	if len(options) > 0 {
		selectedCategory = strings.ToLower(options[0].StringValue())
	}

	userID := i.Member.User.ID
	guildID := i.GuildID

	message := "🤖 **Music Bot Commands**\n\n"

	categories := c.getCategories(selectedCategory)

	for _, category := range categories {
		commands := c.getCommandsForCategory(category, s, guildID, userID)
		if len(commands) == 0 {
			continue
		}

		message += fmt.Sprintf("**%s Commands:**\n", category)

		for _, cmdName := range commands {
			cmdInfo := c.commandRegistry[cmdName]
			permissionText := ""

			if cmdInfo.RequiredLevel != permissions.LevelUser {
				roleName := c.permissionManager.GetRequiredRoleName(cmdInfo.RequiredLevel)
				permissionText = fmt.Sprintf(" *(%s only)*", roleName)
			}

			message += fmt.Sprintf("• `/%s` - %s%s\n", cmdName, cmdInfo.Description, permissionText)
		}
		message += "\n"
	}

	message += "💡 **Tips:**\n"
	message += "• Use `/nowplaying` to see what's currently playing\n"
	message += "• The bot automatically switches between radio and music modes\n"
	message += "• When no music is queued, radio will resume automatically"

	return message
}

func (c *HelpCommand) getCategories(selectedCategory string) []string {
	if selectedCategory != "all" {
		caser := cases.Title(language.Und)
		return []string{caser.String(selectedCategory)}
	}

	return []string{"Utility", "Voice", "Music", "Radio"}
}

func (c *HelpCommand) getCommandsForCategory(category string, s *discordgo.Session, guildID, userID string) []string {
	var commands []string

	for cmdName, cmdInfo := range c.commandRegistry {
		if cmdInfo.Category != category {
			continue
		}

		hasPermission, err := c.permissionManager.HasPermission(s, guildID, userID, cmdInfo.RequiredLevel)
		if err != nil || !hasPermission {
			continue
		}

		commands = append(commands, cmdName)
	}

	sort.Strings(commands)
	return commands
}
