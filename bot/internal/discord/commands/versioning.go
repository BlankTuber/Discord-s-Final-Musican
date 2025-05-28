package commands

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"musicbot/internal/logger"

	"github.com/bwmarrin/discordgo"
)

const CommandHashFile = "command_hashes.json"

type CommandInfo struct {
	Hash string `json:"hash"`
	ID   string `json:"id,omitempty"`
}

type CommandRegistry struct {
	Commands map[string]CommandInfo `json:"commands"`
	Version  int                    `json:"version"`
}

type Versioning struct {
	hashFile string
	registry CommandRegistry
}

func NewVersioning(hashFile string) *Versioning {
	if hashFile == "" {
		hashFile = CommandHashFile
	}

	v := &Versioning{
		hashFile: hashFile,
		registry: CommandRegistry{
			Commands: make(map[string]CommandInfo),
			Version:  1,
		},
	}

	v.loadRegistry()
	return v
}

func (v *Versioning) loadRegistry() {
	if _, err := os.Stat(v.hashFile); os.IsNotExist(err) {
		logger.Info.Printf("Command hash file doesn't exist, will create: %s", v.hashFile)
		return
	}

	data, err := os.ReadFile(v.hashFile)
	if err != nil {
		logger.Error.Printf("Failed to read command hash file: %v", err)
		return
	}

	if len(data) > 0 {
		err = json.Unmarshal(data, &v.registry)
		if err != nil {
			logger.Error.Printf("Failed to parse command hash file: %v", err)
			v.registry = CommandRegistry{
				Commands: make(map[string]CommandInfo),
				Version:  1,
			}
		}
	}

	logger.Info.Printf("Loaded command registry with %d commands (version %d)",
		len(v.registry.Commands), v.registry.Version)
}

func (v *Versioning) saveRegistry() error {
	dir := filepath.Dir(v.hashFile)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	v.registry.Version++

	data, err := json.MarshalIndent(v.registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %w", err)
	}

	err = os.WriteFile(v.hashFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write hash file: %w", err)
	}

	logger.Info.Printf("Saved command registry (version %d)", v.registry.Version)
	return nil
}

func (v *Versioning) calculateCommandHash(cmd Command) (string, error) {
	commandDef := &discordgo.ApplicationCommand{
		Name:        cmd.Name(),
		Description: cmd.Description(),
		Options:     cmd.Options(),
	}

	data, err := json.Marshal(commandDef)
	if err != nil {
		return "", fmt.Errorf("failed to marshal command: %w", err)
	}

	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}

func (v *Versioning) HasCommandChanged(cmd Command) (bool, error) {
	currentHash, err := v.calculateCommandHash(cmd)
	if err != nil {
		return true, err
	}

	cmdName := cmd.Name()
	if storedInfo, exists := v.registry.Commands[cmdName]; exists {
		return storedInfo.Hash != currentHash, nil
	}

	return true, nil
}

func (v *Versioning) UpdateCommandHash(cmd Command, commandID string) error {
	hash, err := v.calculateCommandHash(cmd)
	if err != nil {
		return err
	}

	v.registry.Commands[cmd.Name()] = CommandInfo{
		Hash: hash,
		ID:   commandID,
	}

	return nil
}

func (v *Versioning) RemoveCommand(cmdName string) {
	delete(v.registry.Commands, cmdName)
}

func (v *Versioning) GetStoredCommandID(cmdName string) string {
	if info, exists := v.registry.Commands[cmdName]; exists {
		return info.ID
	}
	return ""
}

func (v *Versioning) GetChangeSummary(commands []Command, existingCommands []*discordgo.ApplicationCommand) ChangeSummary {
	summary := ChangeSummary{
		ToCreate: make([]Command, 0),
		ToUpdate: make(map[string]Command),
		ToDelete: make([]string, 0),
	}

	currentCmds := make(map[string]Command)
	for _, cmd := range commands {
		currentCmds[cmd.Name()] = cmd
	}

	existingCmds := make(map[string]*discordgo.ApplicationCommand)
	for _, cmd := range existingCommands {
		existingCmds[cmd.Name] = cmd
	}

	// Check for new and updated commands
	for _, cmd := range commands {
		cmdName := cmd.Name()

		if existingCmd, exists := existingCmds[cmdName]; exists {
			// Command exists, check if it changed
			changed, err := v.HasCommandChanged(cmd)
			if err != nil {
				logger.Error.Printf("Error checking if command %s changed: %v", cmdName, err)
				// If we can't determine if it changed, update it to be safe
				summary.ToUpdate[existingCmd.ID] = cmd
			} else if changed {
				logger.Debug.Printf("Command %s has changes", cmdName)
				summary.ToUpdate[existingCmd.ID] = cmd
			} else {
				logger.Debug.Printf("Command %s is unchanged", cmdName)
			}
		} else {
			// New command
			logger.Debug.Printf("Command %s is new", cmdName)
			summary.ToCreate = append(summary.ToCreate, cmd)
		}
	}

	// Check for deleted commands
	for _, existingCmd := range existingCommands {
		if _, exists := currentCmds[existingCmd.Name]; !exists {
			logger.Debug.Printf("Command %s should be deleted", existingCmd.Name)
			summary.ToDelete = append(summary.ToDelete, existingCmd.ID)
			// Also remove from our registry
			v.RemoveCommand(existingCmd.Name)
		}
	}

	return summary
}

func (v *Versioning) Save() error {
	return v.saveRegistry()
}

type ChangeSummary struct {
	ToCreate []Command
	ToUpdate map[string]Command
	ToDelete []string
}

func (cs ChangeSummary) HasChanges() bool {
	return len(cs.ToCreate) > 0 || len(cs.ToUpdate) > 0 || len(cs.ToDelete) > 0
}

func (cs ChangeSummary) LogSummary() {
	if !cs.HasChanges() {
		logger.Info.Println("No command changes detected")
		return
	}

	logger.Info.Printf("Command changes detected:")
	if len(cs.ToCreate) > 0 {
		logger.Info.Printf("  - Creating %d new commands", len(cs.ToCreate))
		for _, cmd := range cs.ToCreate {
			logger.Info.Printf("    + %s", cmd.Name())
		}
	}

	if len(cs.ToUpdate) > 0 {
		logger.Info.Printf("  - Updating %d commands", len(cs.ToUpdate))
		for _, cmd := range cs.ToUpdate {
			logger.Info.Printf("    ~ %s", cmd.Name())
		}
	}

	if len(cs.ToDelete) > 0 {
		logger.Info.Printf("  - Deleting %d commands", len(cs.ToDelete))
	}
}
