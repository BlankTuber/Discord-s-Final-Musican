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

	DEFAULT_GUILD_ID string `json:"default_guild_id"`
	DEFAULT_VC_ID    string `json:"default_vc_id"`
	RADIO_URL        string `json:"radio_url"`
	IDLE_TIMEOUT     int    `json:"idle_timeout"`
	UDS_PATH         string `json:"uds_path"`
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
	
	if config.RADIO_URL == "" {
		config.RADIO_URL = "https://listen.moe/stream"
	}
	
	if config.IDLE_TIMEOUT == 0 {
		config.IDLE_TIMEOUT = 30
	}
	
	if config.UDS_PATH == "" {
		config.UDS_PATH = "/tmp/downloader.sock"
	}
	
	return config, nil
}