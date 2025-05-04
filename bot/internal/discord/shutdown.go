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

func (sm *ShutdownManager) Shutdown(ctx context.Context) error {
    logger.InfoLogger.Println("Initiating graceful shutdown...")

    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    sm.client.DisableCommands()

    if sm.client.idleCheckTicker != nil {
        sm.client.idleCheckTicker.Stop()
    }

    if sm.client.radioStreamer != nil {
        sm.client.radioStreamer.Stop()
    }

    // Step 3: Use the waitgroup for synchronization
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

    // Step 4: Disconnect voice channels in a separate goroutine
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
        // Continue to close DB and session even on timeout
        break
    case <-done:
        logger.InfoLogger.Println("All components shut down successfully")
    }

    if sm.client.dbManager != nil {
        if err := sm.client.dbManager.Close(); err != nil {
            logger.ErrorLogger.Printf("Error closing database: %v", err)
        }
    }

    // Final step: Close the Discord session
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