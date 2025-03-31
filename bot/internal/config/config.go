package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	DISCORD_TOKEN string  `json:"discord_token"`
	CLIENT_ID     string  `json:"client_id"`
	VOLUME        float32 `json:"volume"`
}

func Load(configPath string) (Config, error) {
	var config Config
	
	// If no path specified, use default
	if configPath == "" {
		configPath = "config.json"
	}
	
	// Resolve absolute path
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return Config{}, err
	}
	
	// Read the file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return Config{}, err
	}
	
	// Parse JSON
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, err
	}
	
	// Validate required fields
	if config.DISCORD_TOKEN == "" {
		return Config{}, errors.New("discord_token is required in config")
	}
	
	return config, nil
}