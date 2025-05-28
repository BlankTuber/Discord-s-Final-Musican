package state

import "time"

type BotState int

const (
	StateIdle BotState = iota
	StateRadio
	StateDJ
	StateTransitioning
)

type OperationState struct {
	IsJoining   bool
	IsLeaving   bool
	IsStreaming bool
	IsPlaying   bool
}

type VoiceState struct {
	CurrentChannel string
	IdleChannel    string
	IsConnected    bool
}

type RadioState struct {
	CurrentStream string
	Volume        float32
	IsPlaying     bool
}

type MusicState struct {
	CurrentSong   *Song
	IsPlaying     bool
	QueuePosition int
}

type Config struct {
	Token       string
	UDSPath     string
	IdleChannel string
	Volume      float32
	Stream      string
	Streams     []StreamOption
}

type StreamOption struct {
	Name string
	URL  string
}

type Song struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Artist      string    `json:"artist"`
	Duration    int       `json:"duration"`
	FilePath    string    `json:"file_path"`
	URL         string    `json:"url"`
	RequestedBy string    `json:"requested_by"`
	AddedAt     time.Time `json:"added_at"`
}

type QueueItem struct {
	ID       int64 `json:"id"`
	SongID   int64 `json:"song_id"`
	Position int   `json:"position"`
	Song     *Song `json:"song,omitempty"`
}
