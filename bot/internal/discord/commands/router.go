package commands

import (
	"musicbot/internal/logger"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Command interface {
	Name() string
	Description() string
	Options() []*discordgo.ApplicationCommandOption
	Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error
}

type Router struct {
	commands   map[string]Command
	session    *discordgo.Session
	versioning *Versioning
	mu         sync.RWMutex
}

func NewRouter(session *discordgo.Session) *Router {
	return &Router{
		commands:   make(map[string]Command),
		session:    session,
		versioning: NewVersioning(""),
		mu:         sync.RWMutex{},
	}
}

func (r *Router) Register(cmd Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[cmd.Name()] = cmd
}

func (r *Router) Handle(i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	cmdName := i.ApplicationCommandData().Name

	r.mu.RLock()
	cmd, exists := r.commands[cmdName]
	r.mu.RUnlock()

	if !exists {
		logger.Error.Printf("Unknown command: %s", cmdName)
		return
	}

	if err := cmd.Execute(r.session, i); err != nil {
		logger.Error.Printf("Command %s failed: %v", cmdName, err)
	}
}

func (r *Router) UpdateCommands() error {
	logger.Info.Println("Checking for command changes...")

	r.mu.RLock()
	commands := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		commands = append(commands, cmd)
	}
	r.mu.RUnlock()

	existing, err := r.session.ApplicationCommands(r.session.State.User.ID, "")
	if err != nil {
		return err
	}

	changeSummary := r.versioning.GetChangeSummary(commands, existing)
	changeSummary.LogSummary()

	if !changeSummary.HasChanges() {
		logger.Info.Println("All commands are up to date")
		return nil
	}

	logger.Info.Println("Applying command changes...")

	for _, cmdID := range changeSummary.ToDelete {
		logger.Info.Printf("Deleting command ID: %s", cmdID)
		err := r.session.ApplicationCommandDelete(r.session.State.User.ID, "", cmdID)
		if err != nil {
			logger.Error.Printf("Failed to delete command %s: %v", cmdID, err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	for cmdID, cmd := range changeSummary.ToUpdate {
		logger.Info.Printf("Updating command: %s", cmd.Name())

		commandDef := &discordgo.ApplicationCommand{
			Name:        cmd.Name(),
			Description: cmd.Description(),
			Options:     cmd.Options(),
		}

		updatedCmd, err := r.session.ApplicationCommandEdit(r.session.State.User.ID, "", cmdID, commandDef)
		if err != nil {
			logger.Error.Printf("Failed to update command %s: %v", cmd.Name(), err)
			continue
		}

		err = r.versioning.UpdateCommandHash(cmd, updatedCmd.ID)
		if err != nil {
			logger.Error.Printf("Failed to update hash for command %s: %v", cmd.Name(), err)
		}

		time.Sleep(200 * time.Millisecond)
	}

	for _, cmd := range changeSummary.ToCreate {
		logger.Info.Printf("Creating command: %s", cmd.Name())

		commandDef := &discordgo.ApplicationCommand{
			Name:        cmd.Name(),
			Description: cmd.Description(),
			Options:     cmd.Options(),
		}

		createdCmd, err := r.session.ApplicationCommandCreate(r.session.State.User.ID, "", commandDef)
		if err != nil {
			logger.Error.Printf("Failed to create command %s: %v", cmd.Name(), err)
			continue
		}

		err = r.versioning.UpdateCommandHash(cmd, createdCmd.ID)
		if err != nil {
			logger.Error.Printf("Failed to store hash for command %s: %v", cmd.Name(), err)
		}

		time.Sleep(200 * time.Millisecond)
	}

	err = r.versioning.Save()
	if err != nil {
		logger.Error.Printf("Failed to save command registry: %v", err)
	}

	logger.Info.Println("Command update completed successfully")
	return nil
}
