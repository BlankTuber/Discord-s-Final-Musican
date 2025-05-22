package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"quidque.com/discord-musican/internal/config"
	"quidque.com/discord-musican/internal/discord"
	"quidque.com/discord-musican/internal/discord/commands"
	"quidque.com/discord-musican/internal/discord/components"
	"quidque.com/discord-musican/internal/logger"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to the config.json file")
	logLevel := flag.Int("logLevel", logger.LevelInfo, "Log level (0=Error, 1=Warning, 2=Info, 3=Debug)")
	flag.Parse()

	// Setup logger
	logger.Setup(*logLevel)
	logger.InfoLogger.Println("Discord Music Bot starting!")

	// Call janitor executable
	janitorPath := "../janitor/janitor"
	logger.InfoLogger.Println("Running janitor...")
	cmd := exec.Command(janitorPath)
	err := cmd.Run()
	if err != nil {
		logger.WarnLogger.Printf("Failed to run janitor: %v", err)
	} else {
		logger.InfoLogger.Println("Janitor completed successfully")
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create the Discord client
	client, err := discord.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Register commands
	registerCommands(client)

	// Register component handlers
	registerComponentHandlers(client)

	// Create shutdown manager
	shutdownManager := discord.NewShutdownManager(client)

	// Connect to Discord
	logger.InfoLogger.Println("Connecting to Discord...")
	if err := client.Connect(); err != nil {
		log.Fatalf("Failed to connect client: %v", err)
	}

	logger.InfoLogger.Println("Bot is now running. Press CTRL-C to exit.")

	// Handle shutdown signals
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Create shutdown context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initiate graceful shutdown
	logger.InfoLogger.Println("Shutting down...")
	if err := shutdownManager.Shutdown(ctx); err != nil {
		logger.ErrorLogger.Printf("Error during shutdown: %v", err)
		os.Exit(1)
	}
	logger.InfoLogger.Println("Shutdown complete.")
}

func registerCommands(client *discord.Client) {
	// Register music commands
	client.Router.RegisterCommand(commands.NewPlayCommand(client))
	client.Router.RegisterCommand(commands.NewPlaylistCommand(client))
	client.Router.RegisterCommand(commands.NewSearchCommand(client))
	client.Router.RegisterCommand(commands.NewQueueCommand(client))
	client.Router.RegisterCommand(commands.NewSkipCommand(client))
	client.Router.RegisterCommand(commands.NewNowPlayingCommand(client))
	client.Router.RegisterCommand(commands.NewVolumeCommand(client))
	client.Router.RegisterCommand(commands.NewStartCommand(client))
	client.Router.RegisterCommand(commands.NewPauseCommand(client))

	// Register radio commands
	client.Router.RegisterCommand(commands.NewSetDefaultVCCommand(client))
	client.Router.RegisterCommand(commands.NewRadioURLCommand(client))
	client.Router.RegisterCommand(commands.NewRadioVolumeCommand(client))
	client.Router.RegisterCommand(commands.NewRadioStartCommand(client))
	client.Router.RegisterCommand(commands.NewRadioStopCommand(client))

	// Register utility commands
	client.Router.RegisterCommand(commands.NewPingCommand(client))
	client.Router.RegisterCommand(commands.NewHelpCommand(client))

	// Register moderation commands
	client.Router.RegisterCommand(commands.NewClearCommand(client))
	client.Router.RegisterCommand(commands.NewDisconnectUserCommand(client))
	client.Router.RegisterCommand(commands.NewMuteUserCommand(client))
	client.Router.RegisterCommand(commands.NewDeafenUserCommand(client))
}

func registerComponentHandlers(client *discord.Client) {
	// Register search button handler
	client.Router.RegisterComponentHandler(components.NewSearchButtonHandler(client))
}
