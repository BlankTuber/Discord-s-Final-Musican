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
	client            *Client
	voiceConnections  map[string]*discordgo.VoiceConnection
	players           map[string]*audio.Player
	playbackStatus    map[string]audio.PlayerState
	lastKnownChannels map[string]string // NEW: Track last known channel per guild
	currentVolume     float32
	mu                sync.RWMutex
}

func NewVoiceManager(client *Client) *VoiceManager {
	vm := &VoiceManager{
		client:            client,
		voiceConnections:  make(map[string]*discordgo.VoiceConnection),
		players:           make(map[string]*audio.Player),
		playbackStatus:    make(map[string]audio.PlayerState),
		lastKnownChannels: make(map[string]string), // Initialize the new map
		currentVolume:     0.5,
	}

	// Register for player events
	audio.RegisterPlayerEventHandler(vm.handlePlayerEvent)

	return vm
}

func (vm *VoiceManager) handlePlayerEvent(event audio.PlayerEvent) {
	logger.InfoLogger.Printf("Player event received: %s for guild %s", event.Type, event.GuildID)

	if event.Type == "track_end" || event.Type == "track_skipped" {
		go vm.handleTrackFinished(event.GuildID, event.Track)
	}
}

func (vm *VoiceManager) JoinChannel(guildID, channelID string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	vm.client.StartActivity()

	vm.lastKnownChannels[guildID] = channelID

	if vc, ok := vm.voiceConnections[guildID]; ok && vc != nil {
		if vc.ChannelID == channelID {
			return nil
		}

		if player, exists := vm.players[guildID]; exists && player != nil {
			player.Stop()
		}

		vc.Disconnect()
		delete(vm.voiceConnections, guildID)
		time.Sleep(200 * time.Millisecond)
	}

	vc, err := vm.client.Session.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return fmt.Errorf("failed to join voice channel: %w", err)
	}

	vm.voiceConnections[guildID] = vc
	vm.playbackStatus[guildID] = audio.StateStopped

	time.Sleep(100 * time.Millisecond)

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

	logger.InfoLogger.Println("Stopping all audio playback")

	playersCopy := make(map[string]*audio.Player)
	for k, v := range vm.players {
		playersCopy[k] = v
	}

	vm.mu.Unlock()

	for guildID, player := range playersCopy {
		if player != nil {
			player.Stop()
		}

		vm.mu.Lock()
		delete(vm.players, guildID)
		vm.playbackStatus[guildID] = audio.StateStopped
		vm.mu.Unlock()
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

	delete(vm.voiceConnections, guildID)

	if player, exists := vm.players[guildID]; exists && player != nil {
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
	logger.InfoLogger.Printf("StartPlayingFromQueue called for guild %s", guildID)

	vm.mu.Lock()

	vc, ok := vm.voiceConnections[guildID]
	if !ok || vc == nil {
		vm.mu.Unlock()
		logger.ErrorLogger.Printf("Cannot play in guild %s: not connected to voice", guildID)
		return
	}

	if player, exists := vm.players[guildID]; exists && player != nil {
		state := player.GetState()
		if state == audio.StatePlaying || state == audio.StatePaused {
			vm.mu.Unlock()
			logger.InfoLogger.Printf("Player already active in guild %s with state %s", guildID, state)
			return
		}
		player.Stop()
		delete(vm.players, guildID)
		time.Sleep(100 * time.Millisecond)
	}

	volume := vm.currentVolume
	vm.mu.Unlock()

	track := vm.client.QueueManager.GetNextTrack(guildID)
	if track == nil {
		logger.InfoLogger.Printf("No tracks in queue for guild %s", guildID)
		return
	}

	vm.mu.Lock()
	vm.players[guildID] = audio.NewPlayer(vc)
	player := vm.players[guildID]
	player.SetVolume(volume)
	vm.playbackStatus[guildID] = audio.StatePlaying
	vm.mu.Unlock()

	logger.InfoLogger.Printf("Playing track: %s", track.Title)

	player.PlayTrack(track)

	vm.client.Session.UpdateGameStatus(0, fmt.Sprintf("🎵 %s", track.Title))

	vm.client.QueueManager.IncrementPlayCount(track)
}

func (vm *VoiceManager) handleTrackFinished(guildID string, track *audio.Track) {
	logger.InfoLogger.Printf("Track finished: %s in guild %s", track.Title, guildID)

	vm.client.QueueManager.MarkTrackAsPlayed(guildID, track)

	vm.mu.RLock()
	_, isConnected := vm.voiceConnections[guildID]
	vm.mu.RUnlock()

	if !isConnected {
		logger.InfoLogger.Printf("Voice connection lost, cannot play next track")
		return
	}

	nextTrack := vm.client.QueueManager.PeekNextTrack(guildID)
	if nextTrack != nil {
		logger.InfoLogger.Printf("Next track: %s", nextTrack.Title)

		time.Sleep(200 * time.Millisecond)

		vm.StartPlayingFromQueue(guildID)
	} else {
		vm.client.Session.UpdateGameStatus(0, "Queue is empty | Use /play")
		logger.InfoLogger.Printf("Queue is empty for guild %s", guildID)

		go func() {
			time.Sleep(5 * time.Second)
			vm.client.checkIdleState()
		}()
	}
}

// ProtectVoiceConnection marks a voice connection as protected from automatic disconnection
func (vm *VoiceManager) ProtectVoiceConnection(guildID string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// You could add a dedicated protection field here
	// For now we'll use the existing playbackStatus to indicate protection
	vm.playbackStatus[guildID] = audio.StatePlaying

	logger.InfoLogger.Printf("Voice connection for guild %s is now protected", guildID)
}

// GetLastKnownChannel returns the last known voice channel in a guild
func (vm *VoiceManager) GetLastKnownChannel(guildID string) string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	// Try to get from our own tracking
	channelID := vm.lastKnownChannels[guildID]
	return channelID
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
	if !exists || player == nil {
		return false
	}

	// Check if we have a voice connection
	vc, vcExists := vm.voiceConnections[guildID]
	if !vcExists || vc == nil {
		return false
	}

	if player.GetState() != audio.StatePaused {
		return false
	}

	// Update the player's voice connection in case we moved channels
	player.SetVoiceConnection(vc)
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

func (vm *VoiceManager) GetPlayer(guildID string) *audio.Player {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	player, exists := vm.players[guildID]
	if exists {
		return player
	}

	return nil
}
