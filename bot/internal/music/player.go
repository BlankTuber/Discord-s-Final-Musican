package music

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"musicbot/internal/logger"
	"musicbot/internal/state"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"layeh.com/gopus"
)

const (
	frameSize = 960
	channels  = 2
	frameRate = 48000
)

type Player struct {
	stateManager *state.Manager
	stopChan     chan bool
	doneChan     chan struct{}
	isPlaying    bool
	currentSong  *state.Song
	onSongEnd    func()
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.RWMutex
}

func NewPlayer(stateManager *state.Manager) *Player {
	return &Player{
		stateManager: stateManager,
		stopChan:     make(chan bool, 1),
		doneChan:     make(chan struct{}),
	}
}

func (p *Player) SetOnSongEnd(callback func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onSongEnd = callback
}

func (p *Player) Play(vc *discordgo.VoiceConnection, song *state.Song) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isPlaying {
		return fmt.Errorf("already playing a song")
	}

	if _, err := os.Stat(song.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("song file not found: %s", song.FilePath)
	}

	select {
	case <-p.stopChan:
		logger.Debug.Println("Drained leftover stop signal from previous operation")
	default:
	}

	p.doneChan = make(chan struct{})
	p.ctx, p.cancel = context.WithCancel(context.Background())

	p.currentSong = song
	p.stateManager.SetPlaying(true)
	p.isPlaying = true

	logger.Info.Printf("Starting playback: %s by %s", song.Title, song.Artist)

	go p.playLoop(vc, song)

	return nil
}

func (p *Player) Stop() {
	p.mu.Lock()
	if !p.isPlaying {
		p.mu.Unlock()
		return
	}

	logger.Info.Println("Stopping music player...")

	if p.cancel != nil {
		p.cancel()
	}

	select {
	case p.stopChan <- true:
	default:
	}

	doneChan := p.doneChan
	p.mu.Unlock()

	if doneChan != nil {
		select {
		case <-doneChan:
			logger.Debug.Println("Music player stopped successfully")
		case <-time.After(3 * time.Second):
			logger.Error.Println("Timeout waiting for music player to stop")
		}
	}

	p.mu.Lock()
	p.isPlaying = false
	p.currentSong = nil
	p.stateManager.SetPlaying(false)
	p.mu.Unlock()
}

func (p *Player) IsPlaying() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.isPlaying
}

func (p *Player) GetCurrentSong() *state.Song {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentSong
}

func (p *Player) Shutdown(ctx context.Context) error {
	logger.Info.Println("Gracefully shutting down music player...")
	p.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return nil
	}
}

func (p *Player) Name() string {
	return "MusicPlayer"
}

func (p *Player) playLoop(vc *discordgo.VoiceConnection, song *state.Song) {
	defer func() {
		p.mu.RLock()
		doneChan := p.doneChan
		onSongEnd := p.onSongEnd
		p.mu.RUnlock()

		if doneChan != nil {
			close(doneChan)
		}

		if onSongEnd != nil {
			onSongEnd()
		}

		logger.Debug.Println("Music playback goroutine finished")
	}()

	if p.stateManager.IsShuttingDown() {
		logger.Debug.Println("Music player stopping due to shutdown")
		return
	}

	err := p.playFile(vc, song)
	if err != nil {
		if p.stateManager.IsShuttingDown() {
			logger.Debug.Printf("Music playback error during shutdown: %v", err)
		} else {
			logger.Error.Printf("Music playback error: %v", err)
		}
	}
}

func (p *Player) playFile(vc *discordgo.VoiceConnection, song *state.Song) error {
	logger.Debug.Printf("Playing file: %s", song.FilePath)

	ffmpegCtx, ffmpegCancel := context.WithCancel(p.ctx)
	defer ffmpegCancel()

	volume := p.stateManager.GetVolume()

	ffmpeg := exec.CommandContext(ffmpegCtx,
		"ffmpeg",
		"-i", song.FilePath,
		"-f", "s16le",
		"-ar", "48000",
		"-ac", "2",
		"-af", fmt.Sprintf("volume=%f", volume),
		"-loglevel", "error",
		"pipe:1",
	)

	ffmpegOut, err := ffmpeg.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating ffmpeg pipe: %w", err)
	}

	err = ffmpeg.Start()
	if err != nil {
		return fmt.Errorf("error starting ffmpeg: %w", err)
	}

	defer func() {
		if ffmpeg.Process != nil {
			ffmpeg.Process.Kill()
		}
	}()

	vc.Speaking(true)
	defer vc.Speaking(false)

	encoder, err := gopus.NewEncoder(frameRate, channels, gopus.Audio)
	if err != nil {
		return fmt.Errorf("error creating opus encoder: %w", err)
	}

	audioBuf := make([]int16, frameSize*channels)
	opusBuffer := make([]byte, 1000)

	for {
		select {
		case <-p.ctx.Done():
			return nil
		case <-p.stopChan:
			return nil
		default:
		}

		err := binary.Read(ffmpegOut, binary.LittleEndian, &audioBuf)
		if err != nil {
			if err == io.EOF {
				logger.Debug.Printf("Finished playing: %s", song.Title)
				return nil
			}
			return fmt.Errorf("error reading audio data: %w", err)
		}

		opusData, err := encoder.Encode(audioBuf, frameSize, len(opusBuffer))
		if err != nil {
			return fmt.Errorf("error encoding opus: %w", err)
		}

		select {
		case vc.OpusSend <- opusData:
		case <-time.After(2 * time.Second):
			return fmt.Errorf("discord send timeout")
		case <-p.ctx.Done():
			return nil
		}
	}
}
