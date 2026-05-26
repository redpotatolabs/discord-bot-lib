package botlib

import (
	"github.com/bwmarrin/discordgo"
)

// RespondEphemeral sends an ephemeral interaction reply, falling back to
// a followup if the interaction was already acknowledged (the caller
// deferred up front). Same shape as the helper duplicated in vault, tater,
// and notification.
func RespondEphemeral(s *discordgo.Session, i *discordgo.Interaction, content string) {
	if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}); err == nil {
		return
	}
	_, _ = s.FollowupMessageCreate(i, false, &discordgo.WebhookParams{
		Content: content,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
}
