package commands

import (
	"musicbot/internal/music"
	"musicbot/internal/radio"
	"musicbot/internal/state"
	"musicbot/internal/voice"
	"time"

	"github.com/bwmarrin/discordgo"
)

type JoinCommand struct {
	voiceManager *voice.Manager
	radioManager *radio.Manager
	musicManager *music.Manager
	stateManager *state.Manager
}

func NewJoinCommand(voiceManager *voice.Manager, radioManager *radio.Manager, musicManager *music.Manager, stateManager *state.Manager) *JoinCommand {
	return &JoinCommand{
		voiceManager: voiceManager,
		radioManager: radioManager,
		musicManager: musicManager,
		stateManager: stateManager,
	}
}

func (c *JoinCommand) Name() string {
	return "join"
}

func (c *JoinCommand) Description() string {
	return "Join your voice channel"
}

func (c *JoinCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{}
}

func (c *JoinCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		return err
	}

	userVS, err := s.State.VoiceState(i.GuildID, i.Member.User.ID)
	if err != nil || userVS == nil || userVS.ChannelID == "" {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ You need to be in a voice channel."),
		})
		return err
	}

	if c.voiceManager.IsConnectedTo(userVS.ChannelID) {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("✅ Already in your voice channel."),
		})
		return err
	}

	currentState := c.stateManager.GetBotState()

	c.stateManager.SetManualOperationActive(true)
	defer c.stateManager.SetManualOperationActive(false)

	c.musicManager.ExecuteWithDisabledHandlers(func() {
		if currentState == state.StateDJ {
			c.musicManager.Stop()
		} else {
			c.radioManager.Stop()
		}

		time.Sleep(500 * time.Millisecond)

		err = c.voiceManager.JoinUser(i.GuildID, i.Member.User.ID)
		if err != nil {
			return
		}

		time.Sleep(500 * time.Millisecond)

		if c.stateManager.IsInIdleChannel() {
			c.stateManager.SetBotState(state.StateIdle)
			vc := c.voiceManager.GetVoiceConnection()
			if vc != nil && !c.radioManager.IsPlaying() {
				c.radioManager.Start(vc)
			}
		} else {
			if currentState == state.StateDJ && c.musicManager.GetCurrentSong() != nil && !c.musicManager.IsPaused() {
				c.stateManager.SetBotState(state.StateDJ)
				vc := c.voiceManager.GetVoiceConnection()
				if vc != nil {
					c.musicManager.Start(vc)
				}
			} else {
				c.stateManager.SetBotState(state.StateRadio)
				vc := c.voiceManager.GetVoiceConnection()
				if vc != nil && !c.radioManager.IsPlaying() {
					c.radioManager.Start(vc)
				}
			}
		}
	})

	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to join your voice channel."),
		})
		return err
	}

	if c.stateManager.IsInIdleChannel() {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("✅ Joined your voice channel and started radio."),
		})
	} else {
		currentState := c.stateManager.GetBotState()
		if currentState == state.StateDJ {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("✅ Joined your voice channel and resumed music."),
			})
		} else {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: stringPtr("✅ Joined your voice channel and started radio."),
			})
		}
	}

	return err
}

func stringPtr(s string) *string {
	return &s
}
