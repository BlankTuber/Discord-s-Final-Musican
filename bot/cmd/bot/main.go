package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

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
		Token: cfg.DISCORD_TOKEN,
		ClientID: cfg.CLIENT_ID,
		DefaultVolume: cfg.VOLUME,
	})

	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	logger.InfoLogger.Println("Connecting to Discord...")
	if err := client.Connect(); err != nil {
		log.Fatalf("Failed to connect client: %v", err)
	}

	logger.InfoLogger.Println("Bot is now running. Press CTRL-C to exit.")
	
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	logger.InfoLogger.Println("Shutting down...")
	if err := client.Disconnect(); err != nil {
		logger.ErrorLogger.Printf("Error during shutdown: %v", err)
	}
	logger.InfoLogger.Println("Shutdown complete.")
}