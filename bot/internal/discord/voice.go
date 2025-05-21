package discord

import (
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/logger"
)

type VoiceManager struct {
	client           *Client
	voiceConnections map[string]*discordgo.VoiceConnection
	players          map[string]*audio.Player
	playbackStatus   map[string]audio.PlayerState
	currentVolume    float32
	mu               sync.RWMutex
}

func NewVoiceManager(client *Client) *VoiceManager {
	return &VoiceManager{
		client:           client,
		voiceConnections: make(map[string]*discordgo.VoiceConnection),
		players:          make(map[string]*audio.Player),
		playbackStatus:   make(map[string]audio.PlayerState),
		currentVolume:    0.5,
	}
}

func (vm *VoiceManager) JoinChannel(guildID, channelID string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	vm.client.StartActivity()

	if vc, ok := vm.voiceConnections[guildID]; ok {
		if vc.ChannelID == channelID {
			return nil
		}

		// Stop any audio playing in the current voice channel
		if player, exists := vm.players[guildID]; exists && player != nil {
			player.Stop()
		}

		vc.Disconnect()
	}

	vc, err := vm.client.Session.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return fmt.Errorf("failed to join voice channel: %w", err)
	}

	vm.voiceConnections[guildID] = vc
	vm.playbackStatus[guildID] = audio.StateStopped

	return nil
}

func (vm *VoiceManager) LeaveChannel(guildID string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	vc, ok := vm.voiceConnections[guildID]
	if !ok || vc == nil {
		return fmt.Errorf("not connected to a voice channel in guild %s", guildID)
	}

	if player, exists := vm.players[guildID]; exists {
		player.Stop()
		delete(vm.players, guildID)
	}

	if err := vc.Disconnect(); err != nil {
		return fmt.Errorf("failed to disconnect from voice channel: %w", err)
	}

	delete(vm.voiceConnections, guildID)
	delete(vm.playbackStatus, guildID)

	return nil
}

func (vm *VoiceManager) SetVolume(volume float32) {
	if volume < 0.0 || volume > 1.0 {
		return
	}

	vm.mu.Lock()
	vm.currentVolume = volume

	// Update volume for all current players
	for _, player := range vm.players {
		if player != nil {
			player.SetVolume(volume)
		}
	}

	vm.mu.Unlock()
}

func (vm *VoiceManager) GetVolume() float32 {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.currentVolume
}

func (vm *VoiceManager) IsConnected(guildID string) bool {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	vc, ok := vm.voiceConnections[guildID]
	return ok && vc != nil
}

func (vm *VoiceManager) IsConnectedToChannel(guildID, channelID string) bool {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	vc, ok := vm.voiceConnections[guildID]
	return ok && vc != nil && vc.ChannelID == channelID
}

func (vm *VoiceManager) GetConnectedChannel(guildID string) string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	if vc, ok := vm.voiceConnections[guildID]; ok && vc != nil {
		return vc.ChannelID
	}

	return ""
}

func (vm *VoiceManager) GetConnectedChannels() map[string]string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	result := make(map[string]string)
	for guildID, vc := range vm.voiceConnections {
		if vc != nil {
			result[guildID] = vc.ChannelID
		}
	}

	return result
}

func (vm *VoiceManager) StopAllPlayback() {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	logger.InfoLogger.Println("Stopping all audio playback")

	for _, player := range vm.players {
		if player != nil {
			player.Stop()
		}
	}

	vm.players = make(map[string]*audio.Player)

	for guildID := range vm.playbackStatus {
		vm.playbackStatus[guildID] = audio.StateStopped
	}
}

func (vm *VoiceManager) DisconnectAll() {
	vm.mu.Lock()
	guildIDs := make([]string, 0, len(vm.voiceConnections))

	for guildID := range vm.voiceConnections {
		guildIDs = append(guildIDs, guildID)
	}
	vm.mu.Unlock()

	for _, guildID := range guildIDs {
		vm.LeaveChannel(guildID)
	}
}

func (vm *VoiceManager) HandleDisconnect(guildID string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Cleanup on disconnect
	delete(vm.voiceConnections, guildID)

	if player, exists := vm.players[guildID]; exists {
		player.Stop()
		delete(vm.players, guildID)
	}

	delete(vm.playbackStatus, guildID)
}

func (vm *VoiceManager) HandleChannelMove(guildID, newChannelID string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Update our connection to the new channel
	if vc, ok := vm.voiceConnections[guildID]; ok && vc != nil {
		// Save the current state
		vc.ChannelID = newChannelID
	}
}

func (vm *VoiceManager) StartPlayingFromQueue(guildID string) {
	vm.mu.Lock()

	// Make sure we have a voice connection
	vc, ok := vm.voiceConnections[guildID]
	if !ok || vc == nil {
		vm.mu.Unlock()
		logger.ErrorLogger.Printf("Cannot play in guild %s: not connected to voice", guildID)
		return
	}

	// Check if we already have a player
	if player, exists := vm.players[guildID]; exists && player != nil &&
		(player.GetState() == audio.StatePlaying || player.GetState() == audio.StatePaused) {
		vm.mu.Unlock()
		logger.InfoLogger.Printf("Player already active in guild %s", guildID)
		return
	}

	volume := vm.currentVolume
	vm.mu.Unlock()

	// Get the next track from the queue
	track := vm.client.QueueManager.GetNextTrack(guildID)
	if track == nil {
		logger.InfoLogger.Printf("No tracks in queue for guild %s", guildID)
		return
	}

	vm.mu.Lock()
	if _, exists := vm.players[guildID]; !exists {
		// Create a new player
		vm.players[guildID] = audio.NewPlayer(vc)
	} else {
		// Reset existing player
		vm.players[guildID].Stop()
	}

	player := vm.players[guildID]
	player.SetVolume(volume)
	vm.playbackStatus[guildID] = audio.StatePlaying
	vm.mu.Unlock()

	// Start playback
	go func() {
		player.PlayTrack(track)

		// Set track metadata
		vm.client.Session.UpdateGameStatus(0, fmt.Sprintf("🎵 %s", track.Title))

		// Increment play count
		vm.client.QueueManager.IncrementPlayCount(track)

		// When track finishes, check if we should play the next one
		vm.handleTrackFinished(guildID, track)
	}()
}

func (vm *VoiceManager) handleTrackFinished(guildID string, track *audio.Track) {
	vm.mu.RLock()
	player, hasPlayer := vm.players[guildID]
	state := audio.StateStopped

	if hasPlayer {
		state = player.GetState()
	}
	vm.mu.RUnlock()

	// Only continue if the player has stopped (not paused or reset)
	if !hasPlayer || state != audio.StateStopped {
		return
	}

	// Mark the track as played in the database
	vm.client.QueueManager.MarkTrackAsPlayed(guildID, track)

	// Check if there are more tracks in the queue
	nextTrack := vm.client.QueueManager.GetNextTrack(guildID)
	if nextTrack != nil {
		// Start playing the next track
		vm.mu.Lock()
		if player, ok := vm.players[guildID]; ok && player != nil {
			volume := vm.currentVolume
			vm.playbackStatus[guildID] = audio.StatePlaying
			vm.mu.Unlock()

			go func() {
				player.SetVolume(volume)
				player.PlayTrack(nextTrack)

				// Set track metadata
				vm.client.Session.UpdateGameStatus(0, fmt.Sprintf("🎵 %s", nextTrack.Title))

				// Increment play count
				vm.client.QueueManager.IncrementPlayCount(nextTrack)

				// Handle track finished recursively
				vm.handleTrackFinished(guildID, nextTrack)
			}()
		} else {
			vm.mu.Unlock()
		}
	} else {
		// Queue is empty, update status
		vm.client.Session.UpdateGameStatus(0, "Queue is empty | Use /play")
		logger.InfoLogger.Printf("Queue is empty for guild %s", guildID)

		// Wait a bit and then check if we should return to idle mode
		go func() {
			time.Sleep(5 * time.Second)
			vm.client.checkIdleState()
		}()
	}
}

func (vm *VoiceManager) PausePlayback(guildID string) bool {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	player, exists := vm.players[guildID]
	if !exists || player == nil || player.GetState() != audio.StatePlaying {
		return false
	}

	player.Pause()
	vm.playbackStatus[guildID] = audio.StatePaused

	return true
}

func (vm *VoiceManager) ResumePlayback(guildID string) bool {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	player, exists := vm.players[guildID]
	if !exists || player == nil || player.GetState() != audio.StatePaused {
		return false
	}

	player.Resume()
	vm.playbackStatus[guildID] = audio.StatePlaying

	return true
}

func (vm *VoiceManager) SkipTrack(guildID string) bool {
	vm.mu.RLock()
	player, exists := vm.players[guildID]
	vm.mu.RUnlock()

	if !exists || player == nil || player.GetState() != audio.StatePlaying {
		return false
	}

	player.Skip()

	// The handleTrackFinished method will take care of playing the next track
	return true
}

func (vm *VoiceManager) GetPlayerState(guildID string) audio.PlayerState {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	if player, exists := vm.players[guildID]; exists && player != nil {
		return player.GetState()
	}

	return audio.StateStopped
}
