package config

import (
	"encoding/json"
	"musicbot/internal/state"
	"os"
)

type FileConfig struct {
	Token         string `json:"token"`
	UDSPath       string `json:"uds_path"`
	GuildID       string `json:"guild_id"`
	IdleChannel   string `json:"idle_channel"`
	DBPath        string `json:"db_path"`
	DJRoleName    string `json:"dj_role_name"`
	AdminRoleName string `json:"admin_role_name"`
}

func LoadFromFile(path string) (FileConfig, error) {
	var config FileConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}

	err = json.Unmarshal(data, &config)
	if err != nil {
		return config, err
	}

	if config.UDSPath == "" {
		config.UDSPath = "/tmp/downloader.sock"
	}

	if config.DBPath == "" {
		config.DBPath = "bot.db"
	}

	if config.DJRoleName == "" {
		config.DJRoleName = "DJ"
	}

	if config.AdminRoleName == "" {
		config.AdminRoleName = "Admin"
	}

	return config, nil
}

func GetDefaultStreams() []state.StreamOption {
	return []state.StreamOption{
		{Name: "listen.moe", URL: "https://listen.moe/stream"},
		{Name: "listen.moe (kpop)", URL: "https://listen.moe/kpop/stream"},
		{Name: "lofi", URL: "https://streams.ilovemusic.de/iloveradio17.mp3"},
		{Name: "video game music", URL: "https://relay.rainwave.cc/all.ogg?1:Xbo7app8bD"},
	}
}
