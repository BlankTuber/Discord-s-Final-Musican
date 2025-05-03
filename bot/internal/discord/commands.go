package discord

import (
	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/logger"
)

type Command interface {
	Name() string
	
	Description() string
	
	Options() []*discordgo.ApplicationCommandOption
	
	Execute(s *discordgo.Session, i *discordgo.InteractionCreate, client *Client)
}

type CommandRegistry struct {
	client   *Client
	commands map[string]Command
}

func NewCommandRegistry(client *Client) *CommandRegistry {
	return &CommandRegistry{
		client:   client,
		commands: make(map[string]Command),
	}
}

func (r *CommandRegistry) Register(cmd Command) {
	name := cmd.Name()
	r.commands[name] = cmd
	logger.DebugLogger.Printf("Registered command handler for: %s", name)
}

func (r *CommandRegistry) GetCommand(name string) (Command, bool) {
	cmd, exists := r.commands[name]
	return cmd, exists
}

func (r *CommandRegistry) GetAllCommands() []Command {
	cmds := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}
	return cmds
}