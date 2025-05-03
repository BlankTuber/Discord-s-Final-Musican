package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"quidque.com/discord-musican/internal/config"
	"quidque.com/discord-musican/internal/discord"
	"quidque.com/discord-musican/internal/logger"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to the config.json file")
	logLevel := flag.Int("logLevel", logger.LevelInfo, "Log level (0=Error, 1=Warning, 2=Info, 3=Debug)")
	flag.Parse()

	logger.Setup(*logLevel)
	logger.InfoLogger.Println("Discord Music Bot starting!")

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	client, err := discord.NewClient(discord.ClientConfig{
		Token:          cfg.DISCORD_TOKEN,
		ClientID:       cfg.CLIENT_ID,
		DefaultVolume:  cfg.VOLUME,
		DefaultGuildID: cfg.DEFAULT_GUILD_ID,
		DefaultVCID:    cfg.DEFAULT_VC_ID,
		RadioURL:       cfg.RADIO_URL,
		IdleTimeout:    cfg.IDLE_TIMEOUT,
		UDSPath:        cfg.UDS_PATH,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	shutdownManager := discord.NewShutdownManager(client)

	logger.InfoLogger.Println("Connecting to Discord...")
	if err := client.Connect(); err != nil {
		log.Fatalf("Failed to connect client: %v", err)
	}

	// Initialize commands as enabled when the bot starts
	client.CommandsEnabled = true
	
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