package radio

import (
	"fmt"
	"musicbot/internal/state"
)

type StreamManager struct {
	streams []state.StreamOption
}

func NewStreamManager(streams []state.StreamOption) *StreamManager {
	return &StreamManager{
		streams: streams,
	}
}

func (sm *StreamManager) GetStreams() []state.StreamOption {
	return sm.streams
}

func (sm *StreamManager) GetStreamByName(name string) (state.StreamOption, error) {
	for _, stream := range sm.streams {
		if stream.Name == name {
			return stream, nil
		}
	}
	return state.StreamOption{}, fmt.Errorf("stream not found: %s", name)
}

func (sm *StreamManager) GetStreamNames() []string {
	names := make([]string, len(sm.streams))
	for i, stream := range sm.streams {
		names[i] = stream.Name
	}
	return names
}

func (sm *StreamManager) IsValidStream(name string) bool {
	_, err := sm.GetStreamByName(name)
	return err == nil
}
