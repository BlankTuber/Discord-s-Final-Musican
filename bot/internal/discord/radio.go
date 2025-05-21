package discord

import (
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"layeh.com/gopus"
	"quidque.com/discord-musican/internal/logger"
)

const (
	frameSize = 960
	channels  = 2
	frameRate = 48000
)

type RadioManager struct {
	client    *Client
	streamURL string
	volume    float32
	stopChan  chan bool
	isPaused  bool
	isActive  bool
	mu        sync.RWMutex
}

func NewRadioManager(client *Client, streamURL string, volume float32) *RadioManager {
	return &RadioManager{
		client:    client,
		streamURL: streamURL,
		volume:    volume,
		stopChan:  make(chan bool, 1),
		isPaused:  false,
		isActive:  false,
	}
}

func (rm *RadioManager) SetStream(url string) {
	if url == "" {
		return
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	oldURL := rm.streamURL
	rm.streamURL = url

	// Only restart if URL actually changed and stream is active
	if rm.isActive && !rm.isPaused && oldURL != url {
		logger.InfoLogger.Printf("Radio stream URL changed, restarting stream")
		select {
		case rm.stopChan <- true:
		default:
		}

		go func() {
			// Let current stream properly shutdown
			time.Sleep(1 * time.Second)
			rm.Start()
		}()
	}
}

func (rm *RadioManager) SetVolume(volume float32) {
	if volume < 0 || volume > 1.0 {
		return
	}

	rm.mu.Lock()
	oldVolume := rm.volume
	rm.volume = volume
	volumeChanged := oldVolume != volume
	isActive := rm.isActive
	isPaused := rm.isPaused
	rm.mu.Unlock()

	// Only restart if volume actually changed and stream is active
	if isActive && !isPaused && volumeChanged {
		logger.InfoLogger.Printf("Radio volume changed, restarting stream")
		select {
		case rm.stopChan <- true:
		default:
		}

		go func() {
			// Let current stream properly shutdown
			time.Sleep(1 * time.Second)
			rm.Start()
		}()
	}
}

func (rm *RadioManager) streamLoop() {
	for {
		rm.mu.RLock()
		active := rm.isActive
		rm.mu.RUnlock()

		if !active {
			logger.InfoLogger.Println("Radio stream loop ending: not active")
			return
		}

		// Get the voice connection from default guild and channel
		rm.client.Mu.RLock()
		defaultGuildID := rm.client.DefaultGuildID
		defaultVCID := rm.client.DefaultVCID
		rm.client.Mu.RUnlock()

		// Make sure we're connected to the default voice channel
		if !rm.client.VoiceManager.IsConnectedToChannel(defaultGuildID, defaultVCID) {
			err := rm.client.RobustJoinVoiceChannel(defaultGuildID, defaultVCID)
			if err != nil {
				logger.ErrorLogger.Printf("Failed to join default voice channel for radio: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
		}

		// Get the voice connection
		rm.client.VoiceManager.mu.RLock()
		vc, exists := rm.client.VoiceManager.voiceConnections[defaultGuildID]
		rm.client.VoiceManager.mu.RUnlock()

		if !exists || vc == nil {
			logger.ErrorLogger.Println("Cannot start radio stream: not connected to default voice channel")
			time.Sleep(5 * time.Second)
			continue
		}

		rm.mu.RLock()
		streamURL := rm.streamURL
		rm.mu.RUnlock()

		logger.InfoLogger.Printf("Starting radio stream from URL: %s", streamURL)
		err := rm.streamAudio(vc)

		rm.mu.RLock()
		active = rm.isActive
		rm.mu.RUnlock()

		if !active {
			logger.InfoLogger.Println("Radio stream loop ending: no longer active after stream attempt")
			return
		}

		if err != nil {
			logger.ErrorLogger.Printf("Radio stream error: %v", err)
			logger.InfoLogger.Println("Will retry radio stream in 5 seconds...")
			time.Sleep(5 * time.Second)
		}
	}
}

func (rm *RadioManager) Start() {
	rm.mu.Lock()
	if rm.isPaused {
		rm.isPaused = false
		rm.mu.Unlock()
		logger.InfoLogger.Println("Radio streamer resumed from paused state")
		return
	}

	if rm.isActive {
		rm.mu.Unlock()
		logger.InfoLogger.Println("Radio streamer already active, ignoring start request")
		return
	}

	rm.isActive = true
	streamURL := rm.streamURL
	volume := rm.volume
	rm.mu.Unlock()

	logger.InfoLogger.Printf("Starting radio streamer with URL: %s and volume: %.2f", streamURL, volume)

	// IMPORTANT: Removed client method call to prevent circular dependency
	// Stop playback directly instead of calling client methods
	rm.client.VoiceManager.StopAllPlayback()

	go rm.streamLoop()
}

func (rm *RadioManager) Stop() {
	rm.mu.Lock()
	if !rm.isActive {
		rm.mu.Unlock()
		return
	}

	rm.isActive = false
	rm.mu.Unlock()

	select {
	case rm.stopChan <- true:
	default:
		rm.stopChan = make(chan bool, 1)
		rm.stopChan <- true
	}

	// IMPORTANT: Remove any calls that might lead back to DisableIdleMode
	logger.InfoLogger.Println("Radio stream stopped")
}

func (rm *RadioManager) Pause() {
	rm.mu.Lock()
	if !rm.isActive || rm.isPaused {
		rm.mu.Unlock()
		return
	}

	rm.isPaused = true
	rm.mu.Unlock()

	rm.Stop()
}

func (rm *RadioManager) Resume() {
	rm.mu.Lock()
	if !rm.isPaused {
		rm.mu.Unlock()
		return
	}

	rm.isPaused = false
	rm.isActive = true
	rm.mu.Unlock()

	go rm.streamLoop()
}

func (rm *RadioManager) streamAudio(vc *discordgo.VoiceConnection) error {
	client := &http.Client{}

	rm.mu.RLock()
	streamURL := rm.streamURL
	volume := rm.volume
	rm.mu.RUnlock()

	req, err := http.NewRequest("GET", streamURL, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Range", "bytes=0-")
	req.Header.Set("Referer", "https://listen.moe/")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error requesting audio stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("server returned error: %d %s", resp.StatusCode, resp.Status)
	}

	logger.InfoLogger.Println("Connected to audio stream")
	logger.InfoLogger.Println("Content-Type:", resp.Header.Get("Content-Type"))

	vc.Speaking(true)
	defer vc.Speaking(false)

	ffmpeg := exec.Command(
		"ffmpeg",
		"-i", "pipe:0",
		"-f", "s16le",
		"-ar", "48000",
		"-ac", "2",
		"-af", fmt.Sprintf("volume=%f", volume),
		"-loglevel", "warning",
		"pipe:1",
	)

	ffmpeg.Stdin = resp.Body
	ffmpegout, err := ffmpeg.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating ffmpeg stdout pipe: %v", err)
	}

	ffmpeg.Stderr = os.Stderr

	err = ffmpeg.Start()
	if err != nil {
		return fmt.Errorf("error starting ffmpeg: %v", err)
	}
	defer ffmpeg.Process.Kill()

	opusEncoder, err := gopus.NewEncoder(frameRate, channels, gopus.Audio)
	if err != nil {
		return fmt.Errorf("error creating opus encoder: %v", err)
	}

	audioBuf := make([]int16, frameSize*channels)
	opusBuffer := make([]byte, 1000)

	for {
		select {
		case <-rm.stopChan:
			logger.InfoLogger.Println("Radio stream stopped")
			return nil
		default:
		}

		err = binary.Read(ffmpegout, binary.LittleEndian, &audioBuf)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return fmt.Errorf("ffmpeg output ended")
			}
			return fmt.Errorf("error reading from ffmpeg: %v", err)
		}

		opusData, err := opusEncoder.Encode(audioBuf, frameSize, len(opusBuffer))
		if err != nil {
			return fmt.Errorf("error encoding to opus: %v", err)
		}

		vc.OpusSend <- opusData
	}
}

func (rm *RadioManager) IsActive() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.isActive
}

func (rm *RadioManager) IsPlaying() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.isActive && !rm.isPaused
}

func (rm *RadioManager) IsPaused() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.isPaused
}

func (rm *RadioManager) GetURL() string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.streamURL
}

func (rm *RadioManager) GetVolume() float32 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.volume
}
