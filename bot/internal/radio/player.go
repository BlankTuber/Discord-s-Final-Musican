package radio

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"musicbot/internal/logger"
	"musicbot/internal/state"

	"github.com/bwmarrin/discordgo"
	"layeh.com/gopus"
)

const (
	frameSize = 960
	channels  = 2
	frameRate = 48000
)

type ErrorType int

const (
	ErrorEOF ErrorType = iota
	ErrorTimeout
	ErrorRateLimit
	ErrorNetwork
	ErrorOther
)

type StreamError struct {
	Type ErrorType
	Err  error
}

func (se StreamError) Error() string {
	return se.Err.Error()
}

type Player struct {
	stateManager *state.Manager
	stopChan     chan bool
	doneChan     chan struct{}
	isPlaying    bool
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

func (p *Player) Start(vc *discordgo.VoiceConnection) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isPlaying {
		return nil
	}

	// Drain any leftover signals from previous stop operations
	select {
	case <-p.stopChan:
		logger.Debug.Println("Drained leftover stop signal from previous operation")
	default:
	}

	// Create new done channel for this session
	p.doneChan = make(chan struct{})
	p.ctx, p.cancel = context.WithCancel(context.Background())

	p.stateManager.SetStreaming(true)
	p.isPlaying = true

	go p.streamLoop(vc)

	return nil
}

func (p *Player) Stop() {
	p.mu.Lock()
	if !p.isPlaying {
		p.mu.Unlock()
		return
	}

	logger.Info.Println("Stopping radio player...")

	if p.cancel != nil {
		p.cancel()
	}

	select {
	case p.stopChan <- true:
	default:
	}

	// Get reference to done channel before releasing lock
	doneChan := p.doneChan
	p.mu.Unlock()

	// Wait for goroutine to actually finish
	if doneChan != nil {
		select {
		case <-doneChan:
			logger.Debug.Println("Radio player stopped successfully")
		case <-time.After(3 * time.Second):
			logger.Error.Println("Timeout waiting for radio player to stop")
		}
	}

	p.mu.Lock()
	p.isPlaying = false
	p.stateManager.SetStreaming(false)
	p.stateManager.SetRadioPlaying(false)
	p.mu.Unlock()
}

func (p *Player) IsPlaying() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.isPlaying
}

func (p *Player) Shutdown(ctx context.Context) error {
	logger.Info.Println("Gracefully shutting down radio player...")
	p.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return nil
	}
}

func (p *Player) Name() string {
	return "RadioPlayer"
}

func (p *Player) streamLoop(vc *discordgo.VoiceConnection) {
	defer func() {
		// Signal that the goroutine has finished
		p.mu.RLock()
		doneChan := p.doneChan
		p.mu.RUnlock()

		if doneChan != nil {
			close(doneChan)
		}

		logger.Debug.Println("Radio stream goroutine finished")
	}()

	consecutiveNetworkErrors := 0

	for {
		select {
		case <-p.ctx.Done():
			logger.Info.Println("Radio stream context cancelled")
			return
		case <-p.stopChan:
			logger.Info.Println("Radio stream stop requested")
			return
		default:
		}

		if p.stateManager.IsShuttingDown() {
			logger.Debug.Println("Radio stream stopping due to shutdown")
			return
		}

		if !p.stateManager.IsConnected() {
			if p.stateManager.IsShuttingDown() {
				logger.Debug.Println("Not connected to voice during shutdown, stopping radio")
			} else {
				logger.Info.Println("Not connected to voice, stopping radio")
			}
			return
		}

		streamURL := p.stateManager.GetRadioStream()
		volume := p.stateManager.GetVolume()

		err := p.streamAudio(vc, streamURL, volume)

		if err != nil {
			if p.stateManager.IsShuttingDown() {
				logger.Debug.Printf("Radio stream error during shutdown: %v", err)
				return
			}

			streamErr, ok := err.(StreamError)
			if !ok {
				streamErr = StreamError{Type: ErrorOther, Err: err}
			}

			delay := p.getRetryDelay(streamErr, &consecutiveNetworkErrors)

			p.logError(streamErr, delay)

			if delay > 0 {
				select {
				case <-p.ctx.Done():
					return
				case <-time.After(delay):
					continue
				}
			}
		} else {
			consecutiveNetworkErrors = 0
		}
	}
}

func (p *Player) classifyError(err error) StreamError {
	if err == nil {
		return StreamError{Type: ErrorOther, Err: err}
	}

	errStr := strings.ToLower(err.Error())

	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return StreamError{Type: ErrorEOF, Err: err}
	}

	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many requests") {
		return StreamError{Type: ErrorRateLimit, Err: err}
	}

	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		return StreamError{Type: ErrorTimeout, Err: err}
	}

	if strings.Contains(errStr, "network") || strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "refused") || strings.Contains(errStr, "reset") {
		return StreamError{Type: ErrorNetwork, Err: err}
	}

	return StreamError{Type: ErrorOther, Err: err}
}

func (p *Player) getRetryDelay(streamErr StreamError, consecutiveNetworkErrors *int) time.Duration {
	switch streamErr.Type {
	case ErrorEOF:
		logger.Debug.Println("Stream EOF detected (song change/metadata update)")
		return 100 * time.Millisecond

	case ErrorRateLimit:
		logger.Info.Println("Rate limit detected, waiting longer")
		return 30 * time.Second

	case ErrorTimeout:
		return 2 * time.Second

	case ErrorNetwork:
		*consecutiveNetworkErrors++
		if *consecutiveNetworkErrors > 10 {
			return 10 * time.Second
		} else if *consecutiveNetworkErrors > 5 {
			return 5 * time.Second
		}
		return 1 * time.Second

	default:
		return 3 * time.Second
	}
}

func (p *Player) logError(streamErr StreamError, delay time.Duration) {
	switch streamErr.Type {
	case ErrorEOF:
		logger.Debug.Printf("Stream natural break, reconnecting in %v", delay)
	case ErrorRateLimit:
		logger.Error.Printf("Rate limited: %v, waiting %v", streamErr.Err, delay)
	case ErrorTimeout:
		logger.Info.Printf("Stream timeout: %v, retrying in %v", streamErr.Err, delay)
	case ErrorNetwork:
		logger.Error.Printf("Network error: %v, retrying in %v", streamErr.Err, delay)
	default:
		logger.Error.Printf("Stream error: %v, retrying in %v", streamErr.Err, delay)
	}
}

func (p *Player) streamAudio(vc *discordgo.VoiceConnection, streamURL string, volume float32) error {
	logger.Debug.Printf("Connecting to stream: %s", streamURL)

	ctx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			IdleConnTimeout:   30 * time.Second,
			DisableKeepAlives: false,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return p.classifyError(fmt.Errorf("error creating request: %w", err))
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Discord Bot)")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		return p.classifyError(fmt.Errorf("error requesting stream: %w", err))
	}
	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		logger.Error.Printf("Rate limited, Retry-After: %s", retryAfter)
		return StreamError{Type: ErrorRateLimit, Err: fmt.Errorf("rate limited by server")}
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return p.classifyError(fmt.Errorf("server returned error: %d %s", resp.StatusCode, resp.Status))
	}

	logger.Debug.Println("Successfully connected to stream")

	vc.Speaking(true)
	defer vc.Speaking(false)

	ffmpegCtx, ffmpegCancel := context.WithCancel(p.ctx)
	defer ffmpegCancel()

	ffmpeg := exec.CommandContext(ffmpegCtx,
		"ffmpeg",
		"-i", "pipe:0",
		"-f", "s16le",
		"-ar", "48000",
		"-ac", "2",
		"-af", fmt.Sprintf("volume=%f", volume),
		"-loglevel", "error",
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "2",
		"pipe:1",
	)

	ffmpeg.Stdin = resp.Body
	ffmpegOut, err := ffmpeg.StdoutPipe()
	if err != nil {
		return p.classifyError(fmt.Errorf("error creating ffmpeg pipe: %w", err))
	}

	err = ffmpeg.Start()
	if err != nil {
		return p.classifyError(fmt.Errorf("error starting ffmpeg: %w", err))
	}

	defer func() {
		if ffmpeg.Process != nil {
			ffmpeg.Process.Kill()
		}
	}()

	encoder, err := gopus.NewEncoder(frameRate, channels, gopus.Audio)
	if err != nil {
		return p.classifyError(fmt.Errorf("error creating opus encoder: %w", err))
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

		readDone := make(chan error, 1)
		go func() {
			err := binary.Read(ffmpegOut, binary.LittleEndian, &audioBuf)
			readDone <- err
		}()

		select {
		case err := <-readDone:
			if err != nil {
				return p.classifyError(err)
			}
		case <-time.After(5 * time.Second):
			return StreamError{Type: ErrorTimeout, Err: fmt.Errorf("audio read timeout")}
		case <-p.ctx.Done():
			return nil
		}

		opusData, err := encoder.Encode(audioBuf, frameSize, len(opusBuffer))
		if err != nil {
			return p.classifyError(fmt.Errorf("error encoding opus: %w", err))
		}

		select {
		case vc.OpusSend <- opusData:
		case <-time.After(2 * time.Second):
			return StreamError{Type: ErrorTimeout, Err: fmt.Errorf("discord send timeout")}
		case <-p.ctx.Done():
			return nil
		}
	}
}
