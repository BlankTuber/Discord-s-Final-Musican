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
	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/logger"
)

const (
	frameSize = 960
	channels  = 2
)

type RadioStreamer struct {
	client       *Client
	streamURL    string
	volume       float32
	stopChan     chan bool
	isPaused     bool
	isActive     bool
	mu           sync.RWMutex
}

func NewRadioStreamer(client *Client, streamURL string, volume float32) *RadioStreamer {
	return &RadioStreamer{
		client:    client,
		streamURL: streamURL,
		volume:    volume,
		stopChan:  make(chan bool, 1),
		isPaused:  false,
		isActive:  false,
	}
}

func (rs *RadioStreamer) SetStream(url string) {
	if url == "" {
		return
	}
	
	rs.mu.Lock()
	defer rs.mu.Unlock()
	
	oldURL := rs.streamURL
	rs.streamURL = url
	
	// Only restart if URL actually changed and stream is active
	if rs.isActive && !rs.isPaused && oldURL != url {
		logger.InfoLogger.Printf("Radio stream URL changed, restarting stream")
		select {
		case rs.stopChan <- true:
		default:
		}
		
		go func() {
			// Let current stream properly shutdown
			time.Sleep(1 * time.Second)  
			rs.Start()
		}()
	}
}

func (rs *RadioStreamer) SetVolume(volume float32) {
	if volume < 0 || volume > 1.0 {
		return
	}
	
	rs.mu.Lock()
	oldVolume := rs.volume
	rs.volume = volume
	volumeChanged := oldVolume != volume
	isActive := rs.isActive
	isPaused := rs.isPaused
	rs.mu.Unlock()
	
	// Only restart if volume actually changed and stream is active
	if isActive && !isPaused && volumeChanged {
		logger.InfoLogger.Printf("Radio volume changed, restarting stream")
		select {
		case rs.stopChan <- true:
		default:
		}
		
		go func() {
			// Let current stream properly shutdown
			time.Sleep(1 * time.Second)
			rs.Start()
		}()
	}
}

func (rs *RadioStreamer) Start() {
    rs.mu.Lock()
    if rs.isPaused {
        rs.isPaused = false
        rs.mu.Unlock()
        logger.InfoLogger.Println("Radio streamer resumed from paused state")
        return
    }
    
    if rs.isActive {
        rs.mu.Unlock()
        logger.InfoLogger.Println("Radio streamer already active, ignoring start request")
        return
    }
    
    rs.isActive = true
    streamURL := rs.streamURL
    volume := rs.volume
    rs.mu.Unlock()
    
    logger.InfoLogger.Printf("Starting radio streamer with URL: %s and volume: %.2f", streamURL, volume)
    
    // Don't stop all playback here, just ensure we're the only active audio source
    rs.client.mu.Lock()
    for guildID, player := range rs.client.players {
        if player != nil {
            logger.InfoLogger.Printf("Stopping player in guild %s for radio start", guildID)
            player.Stop()
        }
    }
    rs.client.players = make(map[string]*audio.Player)
    rs.client.mu.Unlock()
    
    go rs.streamLoop()
}

func (rs *RadioStreamer) streamLoop() {
    for {
        rs.mu.RLock()
        active := rs.isActive
        rs.mu.RUnlock()
        
        if !active {
            logger.InfoLogger.Println("Radio stream loop ending: not active")
            return
        }
        
        vc, ok := rs.client.GetCurrentVoiceConnection()
        if !ok || vc == nil {
            logger.ErrorLogger.Println("Cannot start radio stream: not connected to a voice channel")
            time.Sleep(5 * time.Second)
            continue
        }
        
        logger.InfoLogger.Printf("Starting radio stream from URL: %s", rs.streamURL)
        err := rs.streamAudio(vc)
        
        rs.mu.RLock()
        active = rs.isActive
        rs.mu.RUnlock()
        
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


func (rs *RadioStreamer) Stop() {
	rs.mu.Lock()
	if !rs.isActive {
		rs.mu.Unlock()
		return
	}
	
	rs.isActive = false
	rs.mu.Unlock()
	
	select {
	case rs.stopChan <- true:
	default:
		rs.stopChan = make(chan bool, 1)
		rs.stopChan <- true
	}
}

func (rs *RadioStreamer) Pause() {
	rs.mu.Lock()
	if !rs.isActive || rs.isPaused {
		rs.mu.Unlock()
		return
	}
	
	rs.isPaused = true
	rs.mu.Unlock()
	
	rs.Stop()
}

func (rs *RadioStreamer) Resume() {
	rs.mu.Lock()
	if !rs.isPaused {
		rs.mu.Unlock()
		return
	}
	
	rs.isPaused = false
	rs.isActive = true
	rs.mu.Unlock()
	
	go rs.streamLoop()
}

func (rs *RadioStreamer) streamAudio(vc *discordgo.VoiceConnection) error {
	client := &http.Client{}
	
	rs.mu.RLock()
	streamURL := rs.streamURL
	volume := rs.volume
	rs.mu.RUnlock()
	
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

	opusEncoder, err := gopus.NewEncoder(48000, channels, gopus.Audio)
	if err != nil {
		return fmt.Errorf("error creating opus encoder: %v", err)
	}

	audioBuf := make([]int16, frameSize*channels)
	opusBuffer := make([]byte, 1000)
	
	for {
		select {
		case <-rs.stopChan:
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