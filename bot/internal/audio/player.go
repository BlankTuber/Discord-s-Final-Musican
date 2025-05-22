package audio

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"layeh.com/gopus"
	"quidque.com/discord-musican/internal/logger"
)

const (
	channels  = 2
	frameRate = 48000
	frameSize = 960
)

type PlayerState string

const (
	StateStopped PlayerState = "stopped"
	StatePlaying PlayerState = "playing"
	StatePaused  PlayerState = "paused"
)

type Player struct {
	mu           sync.Mutex
	vc           *discordgo.VoiceConnection
	stopChan     chan bool
	pauseFlag    bool
	skipFlag     bool
	stream       *os.File
	volumeLevel  float32
	state        PlayerState
	currentTrack *Track
	pausedTrack  *Track
	pausedOffset int64 // Track position when paused
}

type PlayerEvent struct {
	Type        string
	GuildID     string
	Track       *Track
	ElapsedTime int
}

type PlayerEventHandler func(event PlayerEvent)

var eventHandlers []PlayerEventHandler

func RegisterPlayerEventHandler(handler PlayerEventHandler) {
	eventHandlers = append(eventHandlers, handler)
}

func NewPlayer(vc *discordgo.VoiceConnection) *Player {
	return &Player{
		vc:          vc,
		stopChan:    make(chan bool, 1),
		volumeLevel: 0.5,
		state:       StateStopped,
		pauseFlag:   false,
		skipFlag:    false,
	}
}

func (p *Player) SetVoiceConnection(vc *discordgo.VoiceConnection) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vc = vc
}

func (p *Player) SetVolume(volume float32) {
	if volume < 0 || volume > 1 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.volumeLevel = volume
}

func (p *Player) GetState() PlayerState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *Player) GetCurrentTrack() *Track {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentTrack
}

func (p *Player) PlayTrack(track *Track) {
	if track == nil || track.FilePath == "" {
		return
	}

	// Stop any current playback
	p.Stop()

	p.mu.Lock()
	p.currentTrack = track
	p.pauseFlag = false
	p.skipFlag = false
	p.state = StatePlaying
	p.mu.Unlock()

	// Fire event
	guildID := ""
	if p.vc != nil {
		guildID = p.vc.GuildID
	}

	for _, handler := range eventHandlers {
		handler(PlayerEvent{
			Type:    "track_start",
			GuildID: guildID,
			Track:   track,
		})
	}

	// Start playback in a goroutine
	go p.playTrackInternal(track)
}

func (p *Player) playTrackInternal(track *Track) {
	if track.FilePath == "" {
		logger.ErrorLogger.Printf("Track has no file path: %s", track.Title)
		p.fireTrackEndEvent(track, false, 0)
		return
	}

	if _, err := os.Stat(track.FilePath); os.IsNotExist(err) {
		logger.ErrorLogger.Printf("Audio file not found: %s", track.FilePath)
		p.fireTrackEndEvent(track, false, 0)
		return
	}

	file, err := os.Open(track.FilePath)
	if err != nil {
		logger.ErrorLogger.Printf("Failed to open audio file %s: %v", track.FilePath, err)
		p.fireTrackEndEvent(track, false, 0)
		return
	}
	defer file.Close()

	p.mu.Lock()
	p.stream = file
	volume := p.volumeLevel
	vc := p.vc
	p.mu.Unlock()

	if vc == nil {
		logger.ErrorLogger.Printf("Voice connection is nil, cannot play track: %s", track.Title)
		p.fireTrackEndEvent(track, false, 0)
		return
	}

	vc.Speaking(true)
	defer vc.Speaking(false)

	startTime := time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"ffmpeg",
		"-i", track.FilePath,
		"-f", "s16le",
		"-ar", "48000",
		"-ac", "2",
		"-af", fmt.Sprintf("volume=%f", volume),
		"-loglevel", "warning",
		"pipe:1",
	)

	out, err := cmd.StdoutPipe()
	if err != nil {
		logger.ErrorLogger.Printf("Failed to create stdout pipe: %v", err)
		p.fireTrackEndEvent(track, false, 0)
		return
	}

	err = cmd.Start()
	if err != nil {
		logger.ErrorLogger.Printf("Failed to start ffmpeg: %v", err)
		p.fireTrackEndEvent(track, false, 0)
		return
	}

	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	encoder, err := gopus.NewEncoder(frameRate, channels, gopus.Audio)
	if err != nil {
		logger.ErrorLogger.Printf("Failed to create Opus encoder: %v", err)
		p.fireTrackEndEvent(track, false, 0)
		return
	}

	audioBuf := make([]int16, frameSize*channels)
	opusBuffer := make([]byte, 1000)

	// Start playback loop
	playing := true
	paused := false
	skipped := false

	for playing {
		// Check control flags
		p.mu.Lock()
		paused = p.pauseFlag
		skipped = p.skipFlag
		p.mu.Unlock()

		if paused {
			// If we're paused, store current track and position
			p.mu.Lock()
			p.state = StatePaused
			p.mu.Unlock()

			// Fire event
			for _, handler := range eventHandlers {
				handler(PlayerEvent{
					Type:    "track_pause",
					GuildID: vc.GuildID,
					Track:   track,
				})
			}

			logger.InfoLogger.Printf("Track paused: %s", track.Title)
			return // IMPORTANT: Just return without firing track_end event
		}

		if skipped {
			// If we're skipping, stop playback
			playing = false
			skipped = true
			continue
		}

		// Read the next frame
		done := make(chan struct{})
		var readErr error

		go func() {
			readErr = binary.Read(out, binary.LittleEndian, &audioBuf)
			close(done)
		}()

		select {
		case <-done:
			if readErr != nil {
				if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
					logger.InfoLogger.Printf("End of file reached for track: %s", track.Title)
					playing = false
					continue
				}
				logger.ErrorLogger.Printf("Error reading audio data: %v", readErr)
				playing = false
				continue
			}
		case <-p.stopChan:
			playing = false

			// Check if we're paused or skipped
			p.mu.Lock()
			paused = p.pauseFlag
			skipped = p.skipFlag
			p.mu.Unlock()

			continue
		case <-time.After(5 * time.Second):
			logger.WarnLogger.Printf("Read timeout for track: %s", track.Title)
			playing = false
			continue
		}

		// Encode the audio frame
		opusData, err := encoder.Encode(audioBuf, frameSize, len(opusBuffer))
		if err != nil {
			logger.ErrorLogger.Printf("Error encoding to opus: %v", err)
			playing = false
			continue
		}

		// Send the audio frame
		select {
		case vc.OpusSend <- opusData:
		case <-time.After(1 * time.Second):
			logger.WarnLogger.Printf("Timeout sending opus data")
			playing = false
		case <-p.stopChan:
			playing = false

			// Check if we're paused or skipped
			p.mu.Lock()
			paused = p.pauseFlag
			skipped = p.skipFlag
			p.mu.Unlock()
		}
	}

	// Calculate elapsed time
	elapsedTime := int(time.Since(startTime).Seconds())

	// Fire the track end event
	p.fireTrackEndEvent(track, skipped, elapsedTime)
}

func (p *Player) fireTrackEndEvent(track *Track, skipped bool, elapsedTime int) {
	p.mu.Lock()

	// Only reset state if we're not paused
	if !p.pauseFlag {
		p.state = StateStopped
		p.currentTrack = nil
		p.stream = nil
	}

	// Reset flags
	skippedFlag := p.skipFlag
	p.skipFlag = false

	// Get guild ID
	guildID := ""
	if p.vc != nil {
		guildID = p.vc.GuildID
	}

	p.mu.Unlock()

	// Fire the appropriate event
	eventType := "track_end"
	if skippedFlag {
		eventType = "track_skipped"
	}

	for _, handler := range eventHandlers {
		handler(PlayerEvent{
			Type:        eventType,
			GuildID:     guildID,
			Track:       track,
			ElapsedTime: elapsedTime,
		})
	}
}

func (p *Player) Pause() {
	p.mu.Lock()
	if p.state != StatePlaying {
		p.mu.Unlock()
		return
	}

	p.pauseFlag = true
	// Store current track for resuming later
	p.pausedTrack = p.currentTrack
	p.state = StatePaused
	p.mu.Unlock()

	select {
	case p.stopChan <- true:
	default:
		// If channel is full, create a new one
		p.stopChan = make(chan bool, 1)
		p.stopChan <- true
	}

	// Don't trigger track_end event - we're pausing, not ending
	logger.InfoLogger.Printf("Track paused: %s", p.pausedTrack.Title)
}

func (p *Player) Resume() {
	p.mu.Lock()

	if p.state != StatePaused || p.pausedTrack == nil {
		p.mu.Unlock()
		return
	}

	track := p.pausedTrack
	p.pausedTrack = nil
	p.pauseFlag = false
	p.state = StatePlaying
	p.mu.Unlock()

	// Resume playback
	logger.InfoLogger.Printf("Resuming playback of: %s", track.Title)

	// In a real implementation, we'd seek to the paused position
	go p.playTrackInternal(track)
}

func (p *Player) Skip() {
	p.mu.Lock()

	if p.state == StateStopped {
		p.mu.Unlock()
		return
	}

	p.skipFlag = true
	p.pauseFlag = false
	p.mu.Unlock()

	select {
	case p.stopChan <- true:
	default:
		// If channel is full, create a new one
		p.stopChan = make(chan bool, 1)
		p.stopChan <- true
	}
}

func (p *Player) Stop() {
	p.mu.Lock()

	// Only do something if we're playing or paused
	if p.state == StateStopped {
		p.mu.Unlock()
		return
	}

	p.skipFlag = false
	p.pauseFlag = false
	p.state = StateStopped
	p.pausedTrack = nil
	p.mu.Unlock()

	// Signal the playback goroutine to stop
	select {
	case p.stopChan <- true:
	default:
		// If channel is full, create a new one and signal
		p.stopChan = make(chan bool, 1)
		p.stopChan <- true
	}

	// Wait a bit to ensure everything has stopped
	time.Sleep(100 * time.Millisecond)
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state == StatePlaying
}

func (p *Player) IsPaused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state == StatePaused
}
