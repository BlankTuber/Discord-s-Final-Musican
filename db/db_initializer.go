package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	var sharedFolder string
	flag.StringVar(&sharedFolder, "path", "../shared", "Path to the shared folder")
	flag.Parse()
	
	// Ensure we're using absolute path
	absPath, err := filepath.Abs(sharedFolder)
	if err != nil {
		fmt.Printf("Error resolving absolute path: %v\n", err)
		return
	}
	sharedFolder = absPath
	
	fmt.Printf("Using shared folder: %s\n", sharedFolder)
	
	os.MkdirAll(sharedFolder, 0755)
	
	dbPath := filepath.Join(sharedFolder, "musicbot.db")
	
	_, err = os.Stat(dbPath)
	if err == nil {
		fmt.Printf("Database already exists at %s\n", dbPath)
		fmt.Println("Do you want to recreate it? (y/n)")
		
		var response string
		fmt.Scanln(&response)
		
		if response != "y" && response != "Y" {
			fmt.Println("Database initialization canceled.")
			return
		}
		
		os.Remove(dbPath)
	}
	
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		return
	}
	defer db.Close()
	
	_, err = db.Exec(`
	CREATE TABLE songs (
		id INTEGER PRIMARY KEY,
		title TEXT NOT NULL,
		url TEXT UNIQUE NOT NULL,
		platform TEXT NOT NULL,
		file_path TEXT NOT NULL,
		duration INTEGER,
		file_size INTEGER,
		thumbnail_url TEXT,
		artist TEXT,
		download_date INTEGER NOT NULL,
		play_count INTEGER DEFAULT 0,
		last_played INTEGER,
		is_stream BOOLEAN DEFAULT 0
	)
	`)
	if err != nil {
		fmt.Printf("Error creating songs table: %v\n", err)
		return
	}
	
	_, err = db.Exec(`
	CREATE TABLE playlists (
		id INTEGER PRIMARY KEY,
		title TEXT NOT NULL,
		url TEXT UNIQUE NOT NULL,
		platform TEXT NOT NULL,
		download_date INTEGER NOT NULL
	)
	`)
	if err != nil {
		fmt.Printf("Error creating playlists table: %v\n", err)
		return
	}
	
	_, err = db.Exec(`
	CREATE TABLE playlist_songs (
		playlist_id INTEGER,
		song_id INTEGER,
		position INTEGER,
		FOREIGN KEY (playlist_id) REFERENCES playlists(id),
		FOREIGN KEY (song_id) REFERENCES songs(id),
		PRIMARY KEY (playlist_id, song_id)
	)
	`)
	if err != nil {
		fmt.Printf("Error creating playlist_songs table: %v\n", err)
		return
	}
	
	_, err = db.Exec(`
	CREATE INDEX idx_songs_url ON songs(url);
	CREATE INDEX idx_songs_play_count ON songs(play_count);
	CREATE INDEX idx_songs_last_played ON songs(last_played);
	CREATE INDEX idx_playlists_url ON playlists(url);
	`)
	if err != nil {
		fmt.Printf("Error creating indexes: %v\n", err)
		return
	}
	
	fmt.Printf("Database initialized successfully at: %s\n", dbPath)
}