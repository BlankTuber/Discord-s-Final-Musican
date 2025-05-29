package commands

import (
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type DelMsgCommand struct {
	session *discordgo.Session
}

func NewDelMsgCommand(session *discordgo.Session) *DelMsgCommand {
	return &DelMsgCommand{
		session: session,
	}
}

func (c *DelMsgCommand) Name() string {
	return "delmsg"
}

func (c *DelMsgCommand) Description() string {
	return "Bulk delete messages in the current channel"
}

func (c *DelMsgCommand) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "count",
			Description: "Number of messages to delete (1-98)",
			Required:    true,
			MinValue:    func() *float64 { v := 1.0; return &v }(),
			MaxValue:    98,
		},
	}
}

func (c *DelMsgCommand) Execute(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		return err
	}

	count := int(i.ApplicationCommandData().Options[0].IntValue())
	channelID := i.ChannelID

	messages, err := s.ChannelMessages(channelID, count, "", "", "")
	if err != nil {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ Failed to fetch messages."),
		})
		return err
	}

	if len(messages) == 0 {
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: stringPtr("❌ No messages found to delete."),
		})
		return err
	}

	cutoff := time.Now().Add(-14 * 24 * time.Hour)
	var bulkDeleteIDs []string
	var manualDeleteIDs []string

	for _, msg := range messages {
		if msg.Timestamp.After(cutoff) {
			bulkDeleteIDs = append(bulkDeleteIDs, msg.ID)
		} else {
			manualDeleteIDs = append(manualDeleteIDs, msg.ID)
		}
	}

	deletedCount := 0
	var deleteErr error

	if len(bulkDeleteIDs) > 0 {
		if len(bulkDeleteIDs) == 1 {
			err = s.ChannelMessageDelete(channelID, bulkDeleteIDs[0])
			if err == nil {
				deletedCount++
			} else {
				deleteErr = err
			}
		} else {
			err = s.ChannelMessagesBulkDelete(channelID, bulkDeleteIDs)
			if err == nil {
				deletedCount += len(bulkDeleteIDs)
			} else {
				deleteErr = err
			}
		}
	}

	if len(manualDeleteIDs) > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		manualDeleted := 0

		for _, msgID := range manualDeleteIDs {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				err := s.ChannelMessageDelete(channelID, id)
				if err == nil {
					mu.Lock()
					manualDeleted++
					mu.Unlock()
				}
				time.Sleep(100 * time.Millisecond)
			}(msgID)
		}

		wg.Wait()
		deletedCount += manualDeleted
	}

	var responseContent string
	if deleteErr != nil {
		responseContent = fmt.Sprintf("⚠️ Partially completed: deleted %d messages, but encountered errors.", deletedCount)
	} else {
		responseContent = fmt.Sprintf("✅ Successfully deleted %d messages.", deletedCount)
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &responseContent,
	})

	go func() {
		time.Sleep(5 * time.Second)
		s.InteractionResponseDelete(i.Interaction)
	}()

	return err
}
