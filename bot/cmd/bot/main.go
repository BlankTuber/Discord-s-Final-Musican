package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"musicbot/internal/config"
	"musicbot/internal/discord"
	"musicbot/internal/logger"
	"musicbot/internal/permissions"
	"musicbot/internal/shutdown"
	"musicbot/internal/socket"
	"musicbot/internal/state"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to config file")
	logLevel := flag.Int("log", logger.LevelInfo, "Log level")
	flag.Parse()

	logger.Setup(*logLevel)
	logger.Info.Println("Starting Discord Music Bot...")

	shutdownManager := shutdown.NewManager()

	if err := runJanitor(); err != nil {
		logger.Error.Printf("Janitor failed: %v", err)
	}

	fileConfig, err := config.LoadFromFile(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if fileConfig.GuildID == "" {
		log.Fatal("Guild ID is required in config.json")
	}

	dbManager, err := config.NewDatabaseManager(fileConfig.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer dbManager.Close()

	dbConfig, err := dbManager.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load database config: %v", err)
	}

	botConfig := state.Config{
		Token:       fileConfig.Token,
		UDSPath:     fileConfig.UDSPath,
		IdleChannel: fileConfig.IdleChannel,
		Volume:      dbConfig.Volume,
		Stream:      dbConfig.Stream,
		Streams:     dbConfig.Streams,
	}

	stateManager := state.NewManager(botConfig)

	shutdownManager.SetStateManager(stateManager)

	socketClient := socket.NewClient(fileConfig.UDSPath)
	if err := socketClient.Connect(); err != nil {
		logger.Error.Printf("Failed to connect to socket: %v", err)
		logger.Info.Println("Continuing without socket connection...")
	} else {
		logger.Info.Println("Connected to socket")
		shutdownManager.Register(socketClient)
	}

	permConfig := permissions.Config{
		DJRoleName:    fileConfig.DJRoleName,
		AdminRoleName: fileConfig.AdminRoleName,
	}

	discordClient, err := discord.NewClient(fileConfig.Token, stateManager, dbManager, socketClient, permConfig)
	if err != nil {
		log.Fatalf("Failed to create Discord client: %v", err)
	}

	if err := discordClient.Connect(); err != nil {
		log.Fatalf("Failed to connect to Discord: %v", err)
	}

	shutdownManager.Register(discordClient.GetMusicManager())
	shutdownManager.Register(discordClient.GetRadioManager())
	shutdownManager.Register(discordClient.GetVoiceManager())
	shutdownManager.Register(discordClient)

	if err := discordClient.UpdateCommands(); err != nil {
		logger.Error.Printf("Failed to update commands: %v", err)
	} else {
		logger.Info.Println("Commands updated successfully")
	}

	time.Sleep(2 * time.Second)

	if err := discordClient.StartIdleMode(fileConfig.GuildID); err != nil {
		logger.Error.Printf("Failed to start idle mode: %v", err)
	}

	logger.Info.Println("Bot is now running. Press Ctrl+C to exit.")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info.Println("Shutdown signal received...")

	if err := shutdownManager.Shutdown(30 * time.Second); err != nil {
		logger.Error.Printf("Shutdown error: %v", err)
		os.Exit(1)
	}

	logger.Info.Println("Shutdown complete.")
}

func runJanitor() error {
	logger.Info.Println("Running janitor...")

	cmd := exec.Command("../janitor/janitor", "../shared/musicbot.db", "../shared")
	err := cmd.Run()
	if err != nil {
		return err
	}

	logger.Info.Println("Janitor completed successfully")
	return nil
}
