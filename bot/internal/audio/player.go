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
	sync.Mutex
	vc            *discordgo.VoiceConnection
	stopChan      chan bool
	stream        *os.File
	volumeLevel   float32
	state         PlayerState
	currentTrack  *Track
    pausedTrack   *Track
	queue         []*Track
	playbackCount int
	skipFlag      bool // Added flag to indicate skip operation
}

type PlayerEventHandler func(event string, data interface{})

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
		queue:       make([]*Track, 0),
		skipFlag:    false,
	}
}

func (p *Player) QueueTrack(track *Track) {
	p.Lock()
	defer p.Unlock()
	
	p.queue = append(p.queue, track)
	
	if p.state == StateStopped {
		go p.playNextTrack()
	}
}

func (p *Player) ClearQueue() {
	p.Lock()
	defer p.Unlock()
	
	p.queue = make([]*Track, 0)
}

func (p *Player) GetQueue() []*Track {
	p.Lock()
	defer p.Unlock()
	
	queue := make([]*Track, len(p.queue))
	copy(queue, p.queue)
	
	return queue
}

func (p *Player) GetCurrentTrack() *Track {
	p.Lock()
	defer p.Unlock()
	
	return p.currentTrack
}

func (p *Player) Pause() {
    p.Lock()
    defer p.Unlock()
    
    if p.state == StatePlaying {
        // Set state to paused but don't actually stop playback yet
        p.state = StatePaused
        
        // Send stop signal but with skipFlag false
        select {
        case p.stopChan <- true:
            // Message sent successfully
        default:
            // Channel is full, create a new one
            p.stopChan = make(chan bool, 1)
            p.stopChan <- true
        }
        
        logger.InfoLogger.Println("Playback paused")
    }
}

func (p *Player) SetState(state PlayerState) {
    p.Lock()
    defer p.Unlock()
    p.state = state
}


func (p *Player) Skip() {
    p.Lock()
    if p.state != StateStopped {
        p.skipFlag = true // Set the skip flag before stopping
        p.state = StateStopped // Explicitly set state to stopped, not paused
        
        select {
        case p.stopChan <- true:
            // Message sent successfully
        default:
            // Channel is full, create a new one
            p.stopChan = make(chan bool, 1)
            p.stopChan <- true
        }
    }
    p.Unlock()
}




func (p *Player) Stop() {
    p.Lock()
    if p.state != StateStopped {
        p.skipFlag = false // Make sure skip flag is off for regular stops
        
        // Close the stopChan and recreate it to ensure any waiting goroutines proceed
        close(p.stopChan)
        p.stopChan = make(chan bool, 1)
        
        p.state = StateStopped
    }
    p.Unlock()
}

func (p *Player) SetVolume(volume float32) {
	if volume < 0 || volume > 1 {
		return
	}
	
	p.Lock()
	p.volumeLevel = volume
	p.Unlock()
}

func (p *Player) GetState() PlayerState {
	p.Lock()
	defer p.Unlock()
	return p.state
}

func (p *Player) playNextTrack() {
	p.Lock()
	if len(p.queue) == 0 {
		p.state = StateStopped
		p.currentTrack = nil
		p.Unlock()
		
		for _, handler := range eventHandlers {
			handler("queue_end", nil)
		}
		return
	}
	
	track := p.queue[0]
	p.queue = p.queue[1:]
	p.currentTrack = track
	p.state = StatePlaying
	p.skipFlag = false // Reset skip flag
	p.Unlock()
	
	for _, handler := range eventHandlers {
		handler("track_start", track)
	}
	
	p.playTrack(track)
}

func (p *Player) playTrack(track *Track) {
    if track.FilePath == "" {
        logger.ErrorLogger.Printf("Track has no file path: %s", track.Title)
        go p.playNextTrack()
        return
    }
    
    if _, err := os.Stat(track.FilePath); os.IsNotExist(err) {
        logger.ErrorLogger.Printf("Audio file not found: %s", track.FilePath)
        go p.playNextTrack()
        return
    }
    
    file, err := os.Open(track.FilePath)
    if err != nil {
        logger.ErrorLogger.Printf("Failed to open audio file %s: %v", track.FilePath, err)
        go p.playNextTrack()
        return
    }
    defer file.Close()
    
    p.Lock()
    p.stream = file
    p.state = StatePlaying
    stopChan := p.stopChan
    vc := p.vc
    volumeLevel := p.volumeLevel
    p.Unlock()
    
    vc.Speaking(true)
    defer vc.Speaking(false)
    
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    cmd := exec.CommandContext(ctx,
        "ffmpeg",
        "-i", track.FilePath,
        "-f", "s16le",
        "-ar", "48000",
        "-ac", "2",
        "-af", fmt.Sprintf("volume=%f", volumeLevel),
        "-loglevel", "warning",
        "pipe:1",
    )
    
    out, err := cmd.StdoutPipe()
    if err != nil {
        logger.ErrorLogger.Printf("Failed to create stdout pipe: %v", err)
        go p.playNextTrack()
        return
    }
    
    err = cmd.Start()
    if err != nil {
        logger.ErrorLogger.Printf("Failed to start ffmpeg: %v", err)
        go p.playNextTrack()
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
        go p.playNextTrack()
        return
    }
    
    audioBuf := make([]int16, frameSize*channels)
    opusBuffer := make([]byte, 1000)
    
    playing := true
    
    for playing {
        select {
        case <-stopChan:
            playing = false
            continue
        default:
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
            case <-stopChan:
                playing = false
                continue
            case <-time.After(5 * time.Second):
                logger.WarnLogger.Printf("Read timeout for track: %s", track.Title)
                playing = false
                continue
            }
            
            opusData, err := encoder.Encode(audioBuf, frameSize, len(opusBuffer))
            if err != nil {
                logger.ErrorLogger.Printf("Error encoding to opus: %v", err)
                playing = false
                continue
            }
            
            select {
            case vc.OpusSend <- opusData:
            case <-time.After(1 * time.Second):
                logger.WarnLogger.Printf("Timeout sending opus data")
                playing = false
            case <-stopChan:
                playing = false
            }
        }
    }
    
    p.Lock()
    p.playbackCount++
    p.stream = nil
    wasJustPaused := p.state == StatePaused
    skipFlag := p.skipFlag
    currentState := p.state
    
    if wasJustPaused {
        p.pausedTrack = p.currentTrack  // Store the track for later resumption
        p.state = StatePaused
    } else {
        p.state = StateStopped
        if currentState != StateStopped {
            p.Unlock()  // Unlock before event handlers
            
            for _, handler := range eventHandlers {
                handler("track_end", track)
            }
            
            if skipFlag {
                logger.InfoLogger.Printf("Skip detected, playing next track")
                go p.playNextTrack()
            } else {
                logger.InfoLogger.Printf("Track ended naturally or due to error, playing next track")
                go p.playNextTrack()
            }
            return
        }
    }
    p.Unlock()
}

func (p *Player) Resume() {
    p.Lock()
    if p.state == StatePaused && p.pausedTrack != nil {
        track := p.pausedTrack
        p.pausedTrack = nil
        p.state = StatePlaying
        p.Unlock()
        
        logger.InfoLogger.Printf("Resuming playback of: %s", track.Title)
        go p.playTrack(track)
        return
    }
    p.Unlock()
}