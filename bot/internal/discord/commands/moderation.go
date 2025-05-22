package commands

import (
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"quidque.com/discord-musican/internal/discord"
	"quidque.com/discord-musican/internal/logger"
)

var (
	channelLocks      = make(map[string]bool)
	channelLocksMutex sync.Mutex
)


type ClearCommand struct {
	client *discord.Client
}

func NewClearCommand(client *discord.Client) *ClearCommand {
	return &ClearCommand{
		client: client,
	}
}

func (c *ClearCommand) Name() string {
	return "clear"
}

func (c *ClearCommand) Description() string {
	return "Delete a specified number of messages from the channel"
}

func (c *ClearCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "count",
			Description: "Number of messages to delete (1-99)",
			Required:    true,
			MinValue:    floatPtr(1),
			MaxValue:    99,
		},
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "user",
			Description: "Delete messages only from this user",
			Required:    false,
		},
	}
}

func (c *ClearCommand) RequiredPermissions() int64 {
	return discordgo.PermissionManageMessages
}

func (c *ClearCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	
	channelLocksMutex.Lock()
	if locked, exists := channelLocks[i.ChannelID]; exists && locked {
		channelLocksMutex.Unlock()
		
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "⚠️ A clear operation is already in progress in this channel. Please wait until it completes.",
				Flags:   discordgo.MessageFlagsEphemeral, 
			},
		})
		return
	}

	
	channelLocks[i.ChannelID] = true
	channelLocksMutex.Unlock()

	
	defer func() {
		channelLocksMutex.Lock()
		delete(channelLocks, i.ChannelID)
		channelLocksMutex.Unlock()
	}()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	count := int(options[0].IntValue())

	var targetUser string
	if len(options) > 1 && options[1].UserValue(s) != nil {
		targetUser = options[1].UserValue(s).ID
	}

	
	messages, err := s.ChannelMessages(i.ChannelID, count+1, "", "", i.ID) 
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Error fetching messages: %s", err.Error())),
		})
		return
	}

	
	messagesToDelete := make([]string, 0, len(messages))
	oldMessages := make([]string, 0)

	twoWeeksAgo := time.Now().Add(-14 * 24 * time.Hour)

	for _, msg := range messages {
		
		if targetUser != "" && msg.Author.ID != targetUser {
			continue
		}

		
		createdAt, err := discordgo.SnowflakeTimestamp(msg.ID)
		if err == nil && createdAt.Before(twoWeeksAgo) {
			oldMessages = append(oldMessages, msg.ID)
		} else {
			messagesToDelete = append(messagesToDelete, msg.ID)
		}

		
		if len(messagesToDelete)+len(oldMessages) >= count {
			break
		}
	}

	
	deletedCount := 0
	if len(messagesToDelete) > 0 {
		err = s.ChannelMessagesBulkDelete(i.ChannelID, messagesToDelete)
		if err != nil {
			logger.ErrorLogger.Printf("Error bulk deleting messages: %v", err)
		} else {
			deletedCount += len(messagesToDelete)
		}
	}

	
	for _, msgID := range oldMessages {
		err = s.ChannelMessageDelete(i.ChannelID, msgID)
		if err != nil {
			logger.ErrorLogger.Printf("Error deleting message %s: %v", msgID, err)
		} else {
			deletedCount++
		}

		
		if len(oldMessages) > 5 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Successfully deleted %d messages.", deletedCount)),
	})

	
	go func() {
		time.Sleep(5 * time.Second)
		s.InteractionResponseDelete(i.Interaction)
	}()
}


type DisconnectUserCommand struct {
	client *discord.Client
}

func NewDisconnectUserCommand(client *discord.Client) *DisconnectUserCommand {
	return &DisconnectUserCommand{
		client: client,
	}
}

func (c *DisconnectUserCommand) Name() string {
	return "disconnect"
}

func (c *DisconnectUserCommand) Description() string {
	return "Disconnect a user from voice channel"
}

func (c *DisconnectUserCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "user",
			Description: "User to disconnect",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "reason",
			Description: "Reason for disconnecting",
			Required:    false,
		},
	}
}

func (c *DisconnectUserCommand) RequiredPermissions() int64 {
	return discordgo.PermissionVoiceMoveMembers
}

func (c *DisconnectUserCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	targetUser := options[0].UserValue(s)

	var reason string
	if len(options) > 1 {
		reason = options[1].StringValue()
	} else {
		reason = "No reason provided"
	}

	
	voiceState, err := s.State.VoiceState(i.GuildID, targetUser.ID)
	if err != nil || voiceState == nil || voiceState.ChannelID == "" {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ %s is not connected to a voice channel.", targetUser.Username)),
		})
		return
	}

	
	err = s.GuildMemberMove(i.GuildID, targetUser.ID, nil)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to disconnect %s: %s", targetUser.Username, err.Error())),
		})
		return
	}

	logger.InfoLogger.Printf("User %s disconnected %s for reason: %s", i.Member.User.Username, targetUser.Username, reason)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ Disconnected %s from voice channel.\nReason: %s", targetUser.Username, reason)),
	})
}


type MuteUserCommand struct {
	client *discord.Client
}

func NewMuteUserCommand(client *discord.Client) *MuteUserCommand {
	return &MuteUserCommand{
		client: client,
	}
}

func (c *MuteUserCommand) Name() string {
	return "mute"
}

func (c *MuteUserCommand) Description() string {
	return "Server mute a user in voice channels"
}

func (c *MuteUserCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "user",
			Description: "User to mute",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionBoolean,
			Name:        "state",
			Description: "Mute state (true to mute, false to unmute)",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "reason",
			Description: "Reason for muting/unmuting",
			Required:    false,
		},
	}
}

func (c *MuteUserCommand) RequiredPermissions() int64 {
	return discordgo.PermissionVoiceMuteMembers
}

func (c *MuteUserCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	targetUser := options[0].UserValue(s)
	muteState := options[1].BoolValue()

	var reason string
	if len(options) > 2 {
		reason = options[2].StringValue()
	} else {
		reason = "No reason provided"
	}

	
	voiceState, err := s.State.VoiceState(i.GuildID, targetUser.ID)
	if err != nil || voiceState == nil || voiceState.ChannelID == "" {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ %s is not connected to a voice channel.", targetUser.Username)),
		})
		return
	}

	
	err = s.GuildMemberMute(i.GuildID, targetUser.ID, muteState)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to %s %s: %s",
				muteStateString(muteState), targetUser.Username, err.Error())),
		})
		return
	}

	action := muteStateString(muteState)
	logger.InfoLogger.Printf("User %s %s %s for reason: %s",
		i.Member.User.Username, action, targetUser.Username, reason)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ %s %s in voice channels.\nReason: %s",
			capitalizeFirst(action), targetUser.Username, reason)),
	})
}


type DeafenUserCommand struct {
	client *discord.Client
}

func NewDeafenUserCommand(client *discord.Client) *DeafenUserCommand {
	return &DeafenUserCommand{
		client: client,
	}
}

func (c *DeafenUserCommand) Name() string {
	return "deafen"
}

func (c *DeafenUserCommand) Description() string {
	return "Server deafen a user in voice channels"
}

func (c *DeafenUserCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "user",
			Description: "User to deafen",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionBoolean,
			Name:        "state",
			Description: "Deafen state (true to deafen, false to undeafen)",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "reason",
			Description: "Reason for deafening/undeafening",
			Required:    false,
		},
	}
}

func (c *DeafenUserCommand) RequiredPermissions() int64 {
	return discordgo.PermissionVoiceDeafenMembers
}

func (c *DeafenUserCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	options := i.ApplicationCommandData().Options
	targetUser := options[0].UserValue(s)
	deafenState := options[1].BoolValue()

	var reason string
	if len(options) > 2 {
		reason = options[2].StringValue()
	} else {
		reason = "No reason provided"
	}

	
	voiceState, err := s.State.VoiceState(i.GuildID, targetUser.ID)
	if err != nil || voiceState == nil || voiceState.ChannelID == "" {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ %s is not connected to a voice channel.", targetUser.Username)),
		})
		return
	}

	
	err = s.GuildMemberDeafen(i.GuildID, targetUser.ID, deafenState)
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr(fmt.Sprintf("❌ Failed to %s %s: %s",
				deafenStateString(deafenState), targetUser.Username, err.Error())),
		})
		return
	}

	action := deafenStateString(deafenState)
	logger.InfoLogger.Printf("User %s %s %s for reason: %s",
		i.Member.User.Username, action, targetUser.Username, reason)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: stringPtr(fmt.Sprintf("✅ %s %s in voice channels.\nReason: %s",
			capitalizeFirst(action), targetUser.Username, reason)),
	})
}


func muteStateString(state bool) string {
	if state {
		return "muted"
	}
	return "unmuted"
}

func deafenStateString(state bool) string {
	if state {
		return "deafened"
	}
	return "undeafened"
}

func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return fmt.Sprintf("%c%s", s[0]-32, s[1:]) 
}
