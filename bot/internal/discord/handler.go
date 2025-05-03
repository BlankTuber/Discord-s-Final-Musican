package discord

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/logger"
)

const CommandHashFile = "command_hashes.json"

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

	if !c.IsCommandsEnabled() {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Bot is shutting down, commands are temporarily disabled.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

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
	logger.InfoLogger.Println("Starting slash command refresh process...")
	
	registeredCommands := c.commands.GetAllCommands()
	
	currentCommands := make(map[string]string)
	commandDefs := make([]*discordgo.ApplicationCommand, 0, len(registeredCommands))
	
	for _, cmd := range registeredCommands {
		commandDef := &discordgo.ApplicationCommand{
			Name:        cmd.Name(),
			Description: cmd.Description(),
			Options:     cmd.Options(),
		}
		
		commandDefs = append(commandDefs, commandDef)
		
		hash, err := generateCommandHash(commandDef)
		if err != nil {
			logger.WarnLogger.Printf("Failed to generate hash for command %s: %v", cmd.Name(), err)
			continue
		}
		
		currentCommands[cmd.Name()] = hash
	}
	
	previousCommands, err := loadCommandHashes()
	if err != nil {
		logger.WarnLogger.Printf("Failed to load previous command hashes: %v", err)
		previousCommands = make(map[string]string)
	}
	
	existingCommands, err := c.session.ApplicationCommands(c.clientID, "")
	if err != nil {
		logger.ErrorLogger.Printf("Failed to fetch existing commands: %v", err)
		return fmt.Errorf("failed to fetch existing commands: %w", err)
	}
	
	existingCommandIDs := make(map[string]string)
	for _, cmd := range existingCommands {
		existingCommandIDs[cmd.Name] = cmd.ID
	}
	
	commandsToCreate := make([]*discordgo.ApplicationCommand, 0)
	commandsToUpdate := make(map[string]*discordgo.ApplicationCommand)
	commandsToKeep := make(map[string]bool)
	
	for _, commandDef := range commandDefs {
		commandName := commandDef.Name
		commandsToKeep[commandName] = true
		
		if existingID, exists := existingCommandIDs[commandName]; exists {
			if prevHash, ok := previousCommands[commandName]; ok {
				currentHash := currentCommands[commandName]
				if currentHash != prevHash {
					logger.InfoLogger.Printf("Command has changed: %s", commandName)
					commandsToUpdate[existingID] = commandDef
				} else {
					logger.InfoLogger.Printf("Command unchanged: %s", commandName)
				}
			} else {
				logger.InfoLogger.Printf("No previous hash for existing command: %s", commandName)
				commandsToUpdate[existingID] = commandDef
			}
		} else {
			logger.InfoLogger.Printf("New command to create: %s", commandName)
			commandsToCreate = append(commandsToCreate, commandDef)
		}
	}
	
	commandsToDelete := make([]string, 0)
	for _, cmd := range existingCommands {
		if !commandsToKeep[cmd.Name] {
			logger.InfoLogger.Printf("Command to delete: %s", cmd.Name)
			commandsToDelete = append(commandsToDelete, cmd.ID)
		}
	}
	
	logger.InfoLogger.Printf("Commands to create: %d, to update: %d, to delete: %d",
		len(commandsToCreate), len(commandsToUpdate), len(commandsToDelete))
	
	for _, cmdID := range commandsToDelete {
		logger.InfoLogger.Printf("Deleting command ID: %s", cmdID)
		err := c.session.ApplicationCommandDelete(c.clientID, "", cmdID)
		if err != nil {
			logger.WarnLogger.Printf("Failed to delete command '%s': %v", cmdID, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	for cmdID, cmdDef := range commandsToUpdate {
		logger.InfoLogger.Printf("Updating command: %s", cmdDef.Name)
		_, err := c.session.ApplicationCommandEdit(c.clientID, "", cmdID, cmdDef)
		if err != nil {
			logger.WarnLogger.Printf("Failed to update command '%s': %v", cmdDef.Name, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	for _, cmdDef := range commandsToCreate {
		logger.InfoLogger.Printf("Creating command: %s", cmdDef.Name)
		_, err := c.session.ApplicationCommandCreate(c.clientID, "", cmdDef)
		if err != nil {
			logger.WarnLogger.Printf("Failed to create command '%s': %v", cmdDef.Name, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	
	err = saveCommandHashes(currentCommands)
	if err != nil {
		logger.WarnLogger.Printf("Failed to save command hashes: %v", err)
	}
	
	logger.InfoLogger.Println("✓ Slash commands successfully refreshed")
	return nil
}

func generateCommandHash(command *discordgo.ApplicationCommand) (string, error) {
	data, err := json.Marshal(command)
	if err != nil {
		return "", err
	}
	
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}

func loadCommandHashes() (map[string]string, error) {
	hashes := make(map[string]string)
	
	if _, err := os.Stat(CommandHashFile); os.IsNotExist(err) {
		return hashes, nil
	}
	
	data, err := os.ReadFile(CommandHashFile)
	if err != nil {
		return nil, err
	}
	
	if len(data) > 0 {
		err = json.Unmarshal(data, &hashes)
		if err != nil {
			return nil, err
		}
	}
	
	return hashes, nil
}

func saveCommandHashes(hashes map[string]string) error {
	dir := filepath.Dir(CommandHashFile)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	
	data, err := json.MarshalIndent(hashes, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(CommandHashFile, data, 0644)
}