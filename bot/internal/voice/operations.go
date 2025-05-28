package voice

import (
	"fmt"
	"musicbot/internal/state"

	"github.com/bwmarrin/discordgo"
)

type Operations struct {
	connection   *Connection
	stateManager *state.Manager
	session      *discordgo.Session
}

func NewOperations(session *discordgo.Session, stateManager *state.Manager) *Operations {
	return &Operations{
		connection:   NewConnection(session, stateManager),
		stateManager: stateManager,
		session:      session,
	}
}

func (o *Operations) JoinUserChannel(guildID, userID string) error {
	userChannel, err := o.getUserVoiceChannel(guildID, userID)
	if err != nil {
		return fmt.Errorf("user not in voice channel")
	}

	currentChannel := o.stateManager.GetCurrentChannel()
	if currentChannel == userChannel {
		return fmt.Errorf("already in user's channel")
	}

	return o.connection.Join(guildID, userChannel)
}

func (o *Operations) LeaveToIdle(guildID string) error {
	idleChannel := o.stateManager.GetIdleChannel()
	currentChannel := o.stateManager.GetCurrentChannel()

	if currentChannel == idleChannel {
		return fmt.Errorf("already in idle channel")
	}

	if err := o.connection.Leave(); err != nil {
		return err
	}

	return o.connection.Join(guildID, idleChannel)
}

func (o *Operations) ReturnToIdle(guildID string) error {
	idleChannel := o.stateManager.GetIdleChannel()

	if o.stateManager.IsInIdleChannel() {
		return nil
	}

	if err := o.connection.Leave(); err != nil {
		return err
	}

	return o.connection.Join(guildID, idleChannel)
}

func (o *Operations) GetConnection() *Connection {
	return o.connection
}

func (o *Operations) CheckChannelUsers(guildID, channelID string) (int, error) {
	guild, err := o.session.State.Guild(guildID)
	if err != nil {
		return 0, err
	}

	botID := o.session.State.User.ID
	userCount := 0

	for _, vs := range guild.VoiceStates {
		if vs.ChannelID == channelID && vs.UserID != botID {
			userCount++
		}
	}

	return userCount, nil
}

func (o *Operations) getUserVoiceChannel(guildID, userID string) (string, error) {
	vs, err := o.session.State.VoiceState(guildID, userID)
	if err != nil || vs == nil || vs.ChannelID == "" {
		return "", fmt.Errorf("user not in voice channel")
	}

	return vs.ChannelID, nil
}
