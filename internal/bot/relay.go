package bot

import (
	"context"
	"errors"
	"log"

	"github.com/Debcharon/tego/internal/telegram"
)

func (b *Bot) notify(ctx context.Context, chatID int64, text string) {
	if err := b.api.Send(ctx, chatID, text, 0); err != nil {
		log.Printf("optional notification to %d failed: %v", chatID, err)
	}
}

func (b *Bot) message(ctx context.Context, m *telegram.Message, updateID int64) error {
	admin := b.adminID
	if admin == 0 {
		return b.say(ctx, m.Chat.ID, "please_setup_first")
	}
	if m.From.ID == admin {
		if m.ReplyToMessage == nil {
			return b.say(ctx, admin, "reply_to_no_message")
		}
		sender, ok, err := b.store.Sender(m.ReplyToMessage.MessageID)
		if err != nil {
			return err
		}
		if !ok {
			return b.say(ctx, admin, "reply_to_message_no_data")
		}
		if err := b.api.Copy(ctx, sender, m.Chat.ID, m.MessageID); err != nil {
			var apiErr *telegram.APIError
			if errors.As(err, &apiErr) && apiErr.Code == 400 {
				b.notify(ctx, admin, b.text("reply_type_not_supported"))
				return nil
			}
			if errors.As(err, &apiErr) && apiErr.Code == 403 {
				b.notify(ctx, admin, b.text("blocked_alert"))
				return nil
			}
			return err
		}
		if err := b.store.MarkDelivered(updateID); err != nil {
			return err
		}
		if b.store.Preference(admin).Notification {
			p := b.store.Preference(sender)
			b.notify(ctx, admin, formatUser(b.text("reply_message_sent"), p.Name, sender))
		}
		return nil
	}
	if b.store.Preference(m.From.ID).Blocked {
		return b.say(ctx, m.Chat.ID, "be_blocked_alert")
	}
	forwarded, err := b.api.Forward(ctx, admin, m.Chat.ID, m.MessageID)
	if err != nil {
		return err
	}
	if err := b.store.Link(updateID, forwarded.MessageID, m.From.ID); err != nil {
		return err
	}
	if b.store.Preference(m.From.ID).Notification {
		b.notify(ctx, m.Chat.ID, b.text("message_received_notification"))
	}
	return nil
}
