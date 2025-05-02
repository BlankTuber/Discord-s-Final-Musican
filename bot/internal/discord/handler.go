package discord

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/logger"
)

func (c *Client) setupCommandSystem() {
	c.commands = NewCommandRegistry(c)
	
	c.registerAllCommands()
	
	c.session.AddHandler(c.handleInteraction)
	c.session.AddHandler(c.handleMessageCreate)
}

func (c *Client) registerAllCommands() {
	registerPingCommand(c.commands)
	registerRadioCommands(c.commands)
	registerMusicCommands(c.commands)
	
	logger.InfoLogger.Printf("Registered %d commands", len(c.commands.GetAllCommands()))
}

func (c *Client) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	
	cmdName := i.ApplicationCommandData().Name
	
	userName := "unknown"
	if i.Member != nil && i.Member.User != nil {
		userName = i.Member.User.Username
	}
	logger.InfoLogger.Printf("Slash command '%s' executed by user %s", cmdName, userName)
	
	c.StartActivity()
	
	cmd, exists := c.commands.GetCommand(cmdName)
	if !exists {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Unknown command: %s", cmdName),
			},
		})
		logger.WarnLogger.Printf("Received unknown command: %s", cmdName)
		return
	}
	
	cmd.Execute(s, i, c)
}

func (c *Client) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}
	
	c.StartActivity()
}

func (c *Client) RefreshSlashCommands() error {
	logger.InfoLogger.Println("Refreshing slash commands...")
	
	registeredCommands := c.commands.GetAllCommands()
	
	commandDefs := make([]*discordgo.ApplicationCommand, 0, len(registeredCommands))
	for _, cmd := range registeredCommands {
		commandDefs = append(commandDefs, &discordgo.ApplicationCommand{
			Name:        cmd.Name(),
			Description: cmd.Description(),
			Options:     cmd.Options(),
		})
	}

	existingCommands, err := c.session.ApplicationCommands(c.clientID, "")
	if err != nil {
		return fmt.Errorf("failed to fetch existing commands: %w", err)
	}
	
	if len(existingCommands) > 0 {
		logger.InfoLogger.Printf("Found %d existing commands - removing all", len(existingCommands))
		
		for _, cmd := range existingCommands {
			logger.DebugLogger.Printf("Removing command: %s (ID: %s)", cmd.Name, cmd.ID)
			
			err := c.session.ApplicationCommandDelete(c.clientID, "", cmd.ID)
			if err != nil {
				logger.WarnLogger.Printf("Failed to delete command '%s': %v", cmd.Name, err)
			}
			
			time.Sleep(100 * time.Millisecond)
		}
	} else {
		logger.InfoLogger.Println("No existing commands found")
	}
	
	logger.InfoLogger.Printf("Registering %d commands...", len(commandDefs))
	for _, cmd := range commandDefs {
		newCmd, err := c.session.ApplicationCommandCreate(c.clientID, "", cmd)
		if err != nil {
			return fmt.Errorf("failed to create '%s' command: %w", cmd.Name, err)
		}
		logger.DebugLogger.Printf("Registered command: %s (ID: %s)", newCmd.Name, newCmd.ID)
		
		time.Sleep(200 * time.Millisecond)
	}
	
	logger.InfoLogger.Println("✓ Slash commands successfully refreshed")
	return nil
}