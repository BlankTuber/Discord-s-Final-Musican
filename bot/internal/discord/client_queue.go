package discord

import (
	"errors"
	"fmt"
	"os"
	"time"

	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/logger"
)

func (c *Client) AddTrackToQueue(guildID string, track *audio.Track) {
	// Ensure everything is stopped before adding a new track
	c.StopAllPlayback()
	
	c.mu.Lock()
	defer c.mu.Unlock()

	// Make sure queue exists for this guild
	if _, ok := c.songQueues[guildID]; !ok {
		c.songQueues[guildID] = make([]*audio.Track, 0)
	}

	// Add track to queue
	c.songQueues[guildID] = append(c.songQueues[guildID], track)

	// If this is the only song in the queue, start playing
	if len(c.songQueues[guildID]) == 1 {
		c.startPlayer(guildID)
	}

	// Save the queue to the database
	if c.dbManager != nil {
		go func() {
			err := c.dbManager.SaveQueue(guildID, c.songQueues[guildID])
			if err != nil {
				logger.ErrorLogger.Printf("Failed to save queue to database: %v", err)
			}
		}()
	}
}


// GetQueueState returns the current queue and currently playing track
func (c *Client) GetQueueState(guildID string) ([]*audio.Track, *audio.Track) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var queue []*audio.Track
	if q, ok := c.songQueues[guildID]; ok {
		queue = make([]*audio.Track, len(q))
		copy(queue, q)
	} else {
		// Try to load from database
		if c.dbManager != nil {
			var err error
			queue, err = c.dbManager.GetQueue(guildID)
			if err != nil {
				logger.ErrorLogger.Printf("Error loading queue from database: %v", err)
				queue = make([]*audio.Track, 0)
			}

			// Cache the queue
			c.songQueues[guildID] = make([]*audio.Track, len(queue))
			copy(c.songQueues[guildID], queue)
		} else {
			queue = make([]*audio.Track, 0)
		}
	}

	var currentTrack *audio.Track
	if player, ok := c.players[guildID]; ok {
		currentTrack = player.GetCurrentTrack()
	}

	return queue, currentTrack
}

// GetCurrentTrack returns the currently playing track
func (c *Client) GetCurrentTrack(guildID string) *audio.Track {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if player, ok := c.players[guildID]; ok {
		return player.GetCurrentTrack()
	}

	return nil
}

// SkipSong skips the current song and starts the next one
func (c *Client) SkipSong(guildID string) bool {
	c.mu.RLock()
	player, ok := c.players[guildID]
	c.mu.RUnlock()

	if !ok || player.GetState() != audio.StatePlaying {
		return false
	}

	player.Skip()
	return true
}

// ClearQueue clears the queue for the specified guild
func (c *Client) ClearQueue(guildID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.songQueues[guildID] = make([]*audio.Track, 0)

	if player, ok := c.players[guildID]; ok {
		player.ClearQueue()
	}

	// Clear queue in database
	if c.dbManager != nil {
		go func() {
			err := c.dbManager.ClearQueue(guildID)
			if err != nil {
				logger.ErrorLogger.Printf("Failed to clear queue in database: %v", err)
			}
		}()
	}
}

// RemoveFromQueue removes a track from the queue at the specified position
func (c *Client) RemoveFromQueue(guildID string, position int) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	queue, ok := c.songQueues[guildID]
	if !ok || len(queue) == 0 {
		return false, nil
	}

	if position < 0 || position >= len(queue) {
		return false, nil
	}

	// Remove the track
	c.songQueues[guildID] = append(c.songQueues[guildID][:position], c.songQueues[guildID][position+1:]...)

	// Update the database
	if c.dbManager != nil {
		go func() {
			err := c.dbManager.RemoveQueueItem(guildID, position)
			if err != nil {
				logger.ErrorLogger.Printf("Failed to remove queue item from database: %v", err)
			}
		}()
	}

	return true, nil
}

func (c *Client) startPlayer(guildID string) {
	// This function is called with the mutex held

	if len(c.songQueues[guildID]) == 0 {
		return
	}

	// Make sure we have a voice connection
	vc, ok := c.voiceConnections[guildID]
	if !ok || vc == nil {
		logger.ErrorLogger.Printf("Cannot play in guild %s: not connected to voice", guildID)
		return
	}

	// Stop the radio if it's playing
	if c.isInIdleMode {
		c.radioStreamer.Stop()
		c.isInIdleMode = false
		logger.InfoLogger.Println("Radio stopped for track playback")
	}

	// Create a player if we don't have one
	if _, ok := c.players[guildID]; !ok {
		c.players[guildID] = audio.NewPlayer(vc)
	} else {
		// If player exists, make sure it's stopped
		c.players[guildID].Stop()
		logger.InfoLogger.Println("Stopped existing player before starting new track")
	}

	player := c.players[guildID]
	player.SetVolume(c.currentVolume)

	nextTrack := c.songQueues[guildID][0]
	c.songQueues[guildID] = c.songQueues[guildID][1:]

	// Save the queue update to database
	if c.dbManager != nil {
		go func() {
			err := c.dbManager.SaveQueue(guildID, c.songQueues[guildID])
			if err != nil {
				logger.ErrorLogger.Printf("Failed to save queue to database: %v", err)
			}
		}()
	}

	go func() {
		player.QueueTrack(nextTrack)
	}()
}


// handlePlayerEvent handles events from the audio player
func (c *Client) handlePlayerEvent(event string, data interface{}) {
	switch event {
	case "track_start":
		if track, ok := data.(*audio.Track); ok {
			logger.InfoLogger.Printf("Started playing track: %s", track.Title)
			c.session.UpdateGameStatus(0, fmt.Sprintf("🎵 %s", track.Title))

			// Update database play count
			if c.dbManager != nil && track.URL != "" {
				go func() {
					if err := c.dbManager.IncrementPlayCount(track.URL); err != nil {
						logger.ErrorLogger.Printf("Failed to update play count: %v", err)
					}
				}()
			}
		}
	case "track_end":
        if track, ok := data.(*audio.Track); ok {
            logger.InfoLogger.Printf("Finished playing track: %s", track.Title)

            // Create a copy of the song queues to avoid holding the lock for too long
            c.mu.RLock()
            guildsToCheck := make([]string, 0, len(c.players))
            for guildID := range c.players {
                guildsToCheck = append(guildsToCheck, guildID)
            }
            c.mu.RUnlock()

            // Process each guild separately, checking if it needs the next song
            for _, guildID := range guildsToCheck {
                c.mu.RLock()
                player, playerExists := c.players[guildID]
                queueExists := len(c.songQueues[guildID]) > 0
                c.mu.RUnlock()

                if playerExists && player.GetCurrentTrack() == nil && queueExists {
                    c.mu.Lock()
                    if len(c.songQueues[guildID]) > 0 { // Double check queue still has songs
                        c.startPlayer(guildID)
                    }
                    c.mu.Unlock()
                    break // Found the guild that needs the next song
                }
            }
        }
	case "queue_end":
		logger.InfoLogger.Println("Queue ended")
		c.session.UpdateGameStatus(0, "Queue is empty | Use /play")

		// Wait a bit and then check if we should return to idle mode
		go func() {
			time.Sleep(5 * time.Second)
			c.checkIdleState()
		}()
	}
}

// SyncQueueWithDatabase loads the queue from the database if it's not in memory
func (c *Client) SyncQueueWithDatabase(guildID string) error {
	if c.dbManager == nil {
		return errors.New("database manager not available")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If we already have a queue in memory, don't load from database
	if _, ok := c.songQueues[guildID]; ok {
		return nil
	}

	// Load queue from database
	queue, err := c.dbManager.GetQueue(guildID)
	if err != nil {
		return fmt.Errorf("error loading queue from database: %w", err)
	}

	c.songQueues[guildID] = queue
	return nil
}

func (c *Client) BatchAddToQueue(guildID string, tracks []*audio.Track) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Make sure queue exists for this guild
	if _, ok := c.songQueues[guildID]; !ok {
		c.songQueues[guildID] = make([]*audio.Track, 0)
	}
	
	validTracks := 0
	for _, track := range tracks {
		// Skip tracks without file paths
		if track.FilePath == "" {
			logger.WarnLogger.Printf("Skipping track without file path: %s", track.Title)
			continue
		}
		
		// Ensure file exists
		if _, err := os.Stat(track.FilePath); os.IsNotExist(err) {
			logger.WarnLogger.Printf("Skipping track with missing file: %s", track.FilePath)
			continue
		}
		
		// Add track to queue
		c.songQueues[guildID] = append(c.songQueues[guildID], track)
		validTracks++
	}
	
	// Check if we need to start playback
	shouldStartPlayer := false
	if _, exists := c.players[guildID]; !exists {
		shouldStartPlayer = true
	} else if c.players[guildID].GetCurrentTrack() == nil && len(c.songQueues[guildID]) > 0 {
		shouldStartPlayer = true
	}
	
	// Save the queue to the database
	if c.dbManager != nil {
		go func() {
			err := c.dbManager.SaveQueue(guildID, c.songQueues[guildID])
			if err != nil {
				logger.ErrorLogger.Printf("Failed to save queue to database: %v", err)
			}
		}()
	}
	
	// Start playback if needed
	if shouldStartPlayer {
		logger.InfoLogger.Printf("Starting player for guild %s after batch queue add", guildID)
		go c.startPlayer(guildID)
	}
	
	return validTracks
}

func (c *Client) BatchAddTracksToQueue(guildID string, tracks []*audio.Track) {
    c.mu.Lock()
    defer c.mu.Unlock()

    if _, ok := c.songQueues[guildID]; !ok {
        c.songQueues[guildID] = make([]*audio.Track, 0)
    }

    c.songQueues[guildID] = append(c.songQueues[guildID], tracks...)

    currentlyPlaying := false
    if player, exists := c.players[guildID]; exists && player != nil {
        if player.GetState() == audio.StatePlaying {
            currentlyPlaying = true
        }
    }
    
    if !currentlyPlaying && len(c.songQueues[guildID]) > 0 {
        c.startPlayer(guildID)
    }

    if c.dbManager != nil {
        go func() {
            err := c.dbManager.SaveQueue(guildID, c.songQueues[guildID])
            if err != nil {
                logger.ErrorLogger.Printf("Failed to save queue to database: %v", err)
            }
        }()
    }
}