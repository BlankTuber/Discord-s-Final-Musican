package discord

import (
	"musicbot/internal/logger"
	"musicbot/internal/radio"
	"musicbot/internal/state"
	"musicbot/internal/voice"

	"github.com/bwmarrin/discordgo"
)

type EventHandler struct {
	session      *discordgo.Session
	voiceManager *voice.Manager
	radioManager *radio.Manager
	stateManager *state.Manager
}

func NewEventHandler(session *discordgo.Session, voiceManager *voice.Manager, radioManager *radio.Manager, stateManager *state.Manager) *EventHandler {
	return &EventHandler{
		session:      session,
		voiceManager: voiceManager,
		radioManager: radioManager,
		stateManager: stateManager,
	}
}

func (e *EventHandler) HandleReady(s *discordgo.Session, r *discordgo.Ready) {
	logger.Info.Printf("Bot ready as %s", r.User.Username)
	s.UpdateGameStatus(0, "Radio Mode | /join to move me")
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

		e.radioManager.Stop()

		go func() {
			if err := e.voiceManager.HandleDisconnect(v.GuildID); err != nil {
				logger.Error.Printf("Failed to handle disconnect: %v", err)
				return
			}

			if !e.stateManager.IsShuttingDown() {
				e.stateManager.SetBotState(state.StateIdle)
				vc := e.voiceManager.GetVoiceConnection()
				if vc != nil {
					e.radioManager.Start(vc)
				}
			}
		}()
		return
	}

	e.stateManager.SetCurrentChannel(v.ChannelID)

	if e.stateManager.IsInIdleChannel() {
		e.stateManager.SetBotState(state.StateIdle)
	} else {
		e.stateManager.SetBotState(state.StateRadio)
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
				if err := e.voiceManager.HandleUserLeft(v.GuildID, currentChannel); err != nil {
					logger.Error.Printf("Failed to handle user left: %v", err)
				}
			}
		}()
	}
}
