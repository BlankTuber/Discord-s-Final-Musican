package discord

import (
	"musicbot/internal/logger"
	"musicbot/internal/music"
	"musicbot/internal/radio"
	"musicbot/internal/state"
	"musicbot/internal/voice"
	"time"

	"github.com/bwmarrin/discordgo"
)

type EventHandler struct {
	session      *discordgo.Session
	voiceManager *voice.Manager
	radioManager *radio.Manager
	musicManager *music.Manager
	stateManager *state.Manager
}

func NewEventHandler(session *discordgo.Session, voiceManager *voice.Manager, radioManager *radio.Manager, musicManager *music.Manager, stateManager *state.Manager) *EventHandler {
	return &EventHandler{
		session:      session,
		voiceManager: voiceManager,
		radioManager: radioManager,
		musicManager: musicManager,
		stateManager: stateManager,
	}
}

func (e *EventHandler) HandleReady(s *discordgo.Session, r *discordgo.Ready) {
	logger.Info.Printf("Bot ready as %s", r.User.Username)
	s.UpdateGameStatus(0, "Radio Mode | /play for music")
}

func (e *EventHandler) HandleVoiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	if e.stateManager.IsShuttingDown() {
		logger.Debug.Println("Ignoring voice state update during shutdown")
		return
	}

	botID := s.State.User.ID

	if v.UserID == botID {
		e.handleBotVoiceUpdate(v)
		return
	}

	e.handleUserVoiceUpdate(v)
}

func (e *EventHandler) handleBotVoiceUpdate(v *discordgo.VoiceStateUpdate) {
	if v.ChannelID == "" {
		logger.Info.Println("Bot disconnected from voice")

		if e.stateManager.IsShuttingDown() {
			logger.Debug.Println("Bot disconnect expected during shutdown")
			e.voiceManager.HandleDisconnect(v.GuildID)
			return
		}

		e.musicManager.Stop()
		e.radioManager.Stop()

		go func() {
			time.Sleep(500 * time.Millisecond)

			if err := e.voiceManager.HandleDisconnect(v.GuildID); err != nil {
				logger.Error.Printf("Failed to handle disconnect: %v", err)
				return
			}

			if !e.stateManager.IsShuttingDown() {
				time.Sleep(500 * time.Millisecond)
				e.stateManager.SetBotState(state.StateIdle)

				time.Sleep(500 * time.Millisecond)
				vc := e.voiceManager.GetVoiceConnection()
				if vc != nil && !e.radioManager.IsPlaying() {
					e.radioManager.Start(vc)
				}
			}
		}()
		return
	}

	e.stateManager.SetCurrentChannel(v.ChannelID)

	currentState := e.stateManager.GetBotState()
	if e.stateManager.IsInIdleChannel() {
		if currentState == state.StateDJ {
			e.musicManager.Stop()
		}
		e.stateManager.SetBotState(state.StateIdle)
	} else {
		switch currentState {
		case state.StateIdle:
			if e.musicManager.IsPlaying() {
				e.stateManager.SetBotState(state.StateDJ)
			} else {
				e.stateManager.SetBotState(state.StateRadio)
			}
		case state.StateRadio:
		case state.StateDJ:
		}
	}
}

func (e *EventHandler) handleUserVoiceUpdate(v *discordgo.VoiceStateUpdate) {
	currentChannel := e.stateManager.GetCurrentChannel()
	if currentChannel == "" || v.ChannelID == currentChannel {
		return
	}

	if v.BeforeUpdate != nil && v.BeforeUpdate.ChannelID == currentChannel {
		go func() {
			if !e.stateManager.IsShuttingDown() {
				if err := e.handleUserLeft(v.GuildID, currentChannel); err != nil {
					logger.Error.Printf("Failed to handle user left: %v", err)
				}
			}
		}()
	}
}

func (e *EventHandler) handleUserLeft(guildID, channelID string) error {
	err := e.voiceManager.HandleUserLeft(guildID, channelID)
	if err != nil {
		return err
	}

	if e.stateManager.IsInIdleChannel() {
		currentState := e.stateManager.GetBotState()
		if currentState == state.StateDJ {
			e.musicManager.Stop()
			time.Sleep(500 * time.Millisecond)
			e.stateManager.SetBotState(state.StateIdle)

			time.Sleep(500 * time.Millisecond)
			vc := e.voiceManager.GetVoiceConnection()
			if vc != nil && !e.radioManager.IsPlaying() {
				e.radioManager.Start(vc)
			}
		}
	} else {
		currentState := e.stateManager.GetBotState()
		if currentState == state.StateDJ {
			e.musicManager.Stop()
		}

		e.radioManager.Stop()

		time.Sleep(500 * time.Millisecond)

		err = e.voiceManager.ReturnToIdle(guildID)
		if err != nil {
			return err
		}

		e.stateManager.SetBotState(state.StateIdle)

		time.Sleep(500 * time.Millisecond)
		vc := e.voiceManager.GetVoiceConnection()
		if vc != nil && !e.radioManager.IsPlaying() {
			e.radioManager.Start(vc)
		}
	}

	return nil
}
