package discord

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/logger"
)

const CommandHashFile = "command_hashes.json"

type CommandRouter struct {
	client            *Client
	commands          map[string]Command
	componentHandlers map[string]ComponentHandler
	mu                sync.RWMutex
}

type Command interface {
	Name() string
	Description() string
	Options() []*discordgo.ApplicationCommandOption
	Execute(s *discordgo.Session, i *discordgo.InteractionCreate)
}

type ComponentHandler interface {
	Prefix() string
	Handle(s *discordgo.Session, i *discordgo.InteractionCreate)
}

func NewCommandRouter(client *Client) *CommandRouter {
	return &CommandRouter{
		client:            client,
		commands:          make(map[string]Command),
		componentHandlers: make(map[string]ComponentHandler),
	}
}

func (r *CommandRouter) RegisterCommand(cmd Command) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := cmd.Name()
	r.commands[name] = cmd
	logger.DebugLogger.Printf("Registered command: %s", name)
}

func (r *CommandRouter) RegisterComponentHandler(handler ComponentHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := handler.Prefix()
	r.componentHandlers[prefix] = handler
	logger.DebugLogger.Printf("Registered component handler: %s", prefix)
}

func (r *CommandRouter) GetCommand(name string) (Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, exists := r.commands[name]
	return cmd, exists
}

func (r *CommandRouter) GetAllCommands() []Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmds := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}
	return cmds
}

func (r *CommandRouter) HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !r.client.IsCommandsEnabled() {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Bot is shutting down, commands are temporarily disabled.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		r.handleCommandInteraction(s, i)
	case discordgo.InteractionMessageComponent:
		r.handleComponentInteraction(s, i)
	}
}

func (r *CommandRouter) handleCommandInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cmdName := i.ApplicationCommandData().Name

	userName := "unknown"
	if i.Member != nil && i.Member.User != nil {
		userName = i.Member.User.Username
	}
	logger.InfoLogger.Printf("Slash command '%s' executed by user %s", cmdName, userName)

	r.client.StartActivity()

	cmd, exists := r.GetCommand(cmdName)
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

	cmd.Execute(s, i)
}

func (r *CommandRouter) handleComponentInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	r.client.StartActivity()

	customID := i.MessageComponentData().CustomID

	var handlerPrefix string
	for idx, char := range customID {
		if char == ':' {
			handlerPrefix = customID[:idx]
			break
		}
	}

	if handlerPrefix == "" {
		handlerPrefix = customID
	}

	r.mu.RLock()
	handler, exists := r.componentHandlers[handlerPrefix]
	r.mu.RUnlock()

	if !exists {
		logger.WarnLogger.Printf("No handler for component with prefix: %s", handlerPrefix)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Unknown component interaction.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	handler.Handle(s, i)
}

func (r *CommandRouter) RefreshSlashCommands() error {
	logger.InfoLogger.Println("Starting slash command refresh process...")

	registeredCommands := r.GetAllCommands()

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

	existingCommands, err := r.client.Session.ApplicationCommands(r.client.ClientID, "")
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
		err := r.client.Session.ApplicationCommandDelete(r.client.ClientID, "", cmdID)
		if err != nil {
			logger.WarnLogger.Printf("Failed to delete command '%s': %v", cmdID, err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	for cmdID, cmdDef := range commandsToUpdate {
		logger.InfoLogger.Printf("Updating command: %s", cmdDef.Name)
		_, err := r.client.Session.ApplicationCommandEdit(r.client.ClientID, "", cmdID, cmdDef)
		if err != nil {
			logger.WarnLogger.Printf("Failed to update command '%s': %v", cmdDef.Name, err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	for _, cmdDef := range commandsToCreate {
		logger.InfoLogger.Printf("Creating command: %s", cmdDef.Name)
		_, err := r.client.Session.ApplicationCommandCreate(r.client.ClientID, "", cmdDef)
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
