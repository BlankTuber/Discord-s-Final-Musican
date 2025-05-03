package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

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
	queue         []*Track
	playbackCount int
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

func (p *Player) Skip() {
	if p.state == StatePlaying || p.state == StatePaused {
		p.Stop()
		go p.playNextTrack()
	}
}

func (p *Player) Stop() {
	p.Lock()
	if p.state != StateStopped {
		select {
		case p.stopChan <- true:
		default:
		}
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
	
	// Check if file exists
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
	
	cmd := exec.Command(
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
	
	defer cmd.Process.Kill()
	
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
		default:
			err = binary.Read(out, binary.LittleEndian, &audioBuf)
			if err != nil {
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					playing = false
					continue
				}
				logger.ErrorLogger.Printf("Error reading audio data: %v", err)
				playing = false
				continue
			}
			
			opusData, err := encoder.Encode(audioBuf, frameSize, len(opusBuffer))
			if err != nil {
				logger.ErrorLogger.Printf("Error encoding to opus: %v", err)
				playing = false
				continue
			}
			
			vc.OpusSend <- opusData
		}
	}
	
	p.Lock()
	p.playbackCount++
	p.stream = nil
	
	if p.state != StateStopped {
		p.state = StateStopped
		p.Unlock()
		
		for _, handler := range eventHandlers {
			handler("track_end", track)
		}
		
		go p.playNextTrack()
	} else {
		p.Unlock()
	}
}