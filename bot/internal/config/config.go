package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	DISCORD_TOKEN    string  `json:"discord_token"`
	CLIENT_ID        string  `json:"client_id"`
	VOLUME           float32 `json:"volume"`
	DEFAULT_GUILD_ID string  `json:"default_guild_id"`
	DEFAULT_VC_ID    string  `json:"default_vc_id"`
	RADIO_URL        string  `json:"radio_url"`
	IDLE_TIMEOUT     int     `json:"idle_timeout"`
	UDS_PATH         string  `json:"uds_path"`
	DB_PATH          string  `json:"db_path"`
}

func Load(configPath string) (Config, error) {
	var config Config

	if configPath == "" {
		configPath = "config.json"
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return Config{}, err
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, err
	}

	
	if config.DISCORD_TOKEN == "" {
		return Config{}, errors.New("discord_token is required in config")
	}

	if config.CLIENT_ID == "" {
		return Config{}, errors.New("client_id is required in config")
	}

	
	if config.VOLUME == 0 {
		config.VOLUME = 0.5
	}

	if config.RADIO_URL == "" {
		config.RADIO_URL = "https://listen.moe/stream"
	}

	if config.IDLE_TIMEOUT == 0 {
		config.IDLE_TIMEOUT = 30
	}

	if config.UDS_PATH == "" {
		config.UDS_PATH = "/tmp/downloader.sock"
	}

	if config.DB_PATH == "" {
		config.DB_PATH = "../shared/musicbot.db"
	}

	return config, nil
}

func Save(config Config, configPath string) error {
	if configPath == "" {
		configPath = "config.json"
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(absPath, data, 0644)
}
