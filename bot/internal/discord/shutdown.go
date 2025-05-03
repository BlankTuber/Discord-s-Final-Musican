package discord

import (
	"context"
	"sync"
	"time"

	"quidque.com/discord-musican/internal/logger"
)

// ShutdownManager handles graceful shutdown of bot components
type ShutdownManager struct {
	client *Client
	wg     sync.WaitGroup
}

// NewShutdownManager creates a new shutdown manager
func NewShutdownManager(client *Client) *ShutdownManager {
	return &ShutdownManager{
		client: client,
	}
}

// Shutdown coordinates graceful shutdown of all components
func (sm *ShutdownManager) Shutdown(ctx context.Context) error {
	logger.InfoLogger.Println("Initiating graceful shutdown...")

	// Create a timeout context
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Step 1: Stop accepting new commands
	sm.client.DisableCommands()

	// Step 2: Stop the idle checker and radio mode
	if sm.client.idleCheckTicker != nil {
		sm.client.idleCheckTicker.Stop()
	}

	if sm.client.radioStreamer != nil {
		sm.client.radioStreamer.Stop()
	}

	// Step 3: Stop all active players and clear queues
	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()
		sm.client.StopAllPlayback()
		sm.client.mu.Lock()
		for guildID := range sm.client.songQueues {
			sm.client.songQueues[guildID] = nil
		}
		sm.client.mu.Unlock()
	}()

	// Step 4: Disconnect from voice channels
	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()
		sm.disconnectVoiceChannels()
	}()

	// Step 5: Wait for cleanup with timeout
	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		logger.WarnLogger.Println("Shutdown timed out, forcing exit...")
		return ctx.Err()
	case <-done:
		logger.InfoLogger.Println("All components shut down successfully")
	}

	// Step 6: Close database connections
	if sm.client.dbManager != nil {
		if err := sm.client.dbManager.Close(); err != nil {
			logger.ErrorLogger.Printf("Error closing database: %v", err)
		}
	}

	// Step 7: Finally close the Discord session
	err := sm.client.session.Close()
	if err != nil {
		logger.ErrorLogger.Printf("Error closing Discord session: %v", err)
	}

	return err
}

func (sm *ShutdownManager) disconnectVoiceChannels() {
	sm.client.mu.Lock()
	defer sm.client.mu.Unlock()

	for guildID, vc := range sm.client.voiceConnections {
		if vc != nil {
			vc.Disconnect()
			logger.InfoLogger.Printf("Disconnected from voice channel in guild %s", guildID)
		}
		delete(sm.client.voiceConnections, guildID)
	}
}