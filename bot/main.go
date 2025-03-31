package main

import (
	"encoding/json"
	"log"
	"os"
)

type Config struct {
	DISCORD_TOKEN string  `json:"discord_token"`
	CLIENT_ID     string  `json:"client_id"`
	VOLUME        float32 `json:"volume"`
}

func main() {
	var config Config
	
	dat, err := os.ReadFile("./config.json")
	if err != nil {
		log.Fatal(err)
	}
	
	if err := json.Unmarshal(dat, &config); err != nil {
		log.Fatal(err)
	}
	
	
}