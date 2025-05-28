package state

type BotState int

const (
	StateIdle BotState = iota
	StateRadio
	StateTransitioning
)

type OperationState struct {
	IsJoining   bool
	IsLeaving   bool
	IsStreaming bool
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
