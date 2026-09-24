package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Debcharon/tego/internal/telegram"
)

func parseCommand(text, username string) (string, []string, bool) {
	if !strings.HasPrefix(text, "/") {
		return "", nil, false
	}
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", nil, false
	}
	command := strings.TrimPrefix(parts[0], "/")
	if index := strings.IndexByte(command, '@'); index >= 0 {
		if !strings.EqualFold(command[index+1:], username) {
			return "", nil, false
		}
		command = command[:index]
	}
	return strings.ToLower(command), parts[1:], true
}

func formatUser(pattern, name string, id int64) string {
	// Language files retain the old %s placeholders, but plain text avoids Markdown injection from user names.
	pattern = strings.Replace(pattern, "[Link](tg://user?id=%s)", "%s", 1)
	pattern = strings.Replace(pattern, "[链接](tg://user?id=%s)", "%s", 1)
	return fmt.Sprintf(pattern, name, strconv.FormatInt(id, 10))
}

func (b *Bot) replySender(m *telegram.Message) (int64, bool, error) {
	if m.ReplyToMessage == nil {
		return 0, false, nil
	}
	return b.store.Sender(m.ReplyToMessage.MessageID)
}

func (b *Bot) command(ctx context.Context, m *telegram.Message, command string, args []string, updateID int64) error {
	id := m.From.ID
	admin := b.adminID
	switch command {
	case "start":
		return b.say(ctx, m.Chat.ID, "start")
	case "help":
		return b.api.Send(ctx, m.Chat.ID, "tego\n"+b.version+"\nhttps://github.com/Debcharon/tego", 0)
	case "ping":
		return b.api.Send(ctx, m.Chat.ID, "Pong!", 0)
	case "notification":
		p := b.store.Preference(id)
		p.Notification = !p.Notification
		if err := b.store.SetPreferenceForUpdate(updateID, id, p); err != nil {
			return err
		}
		if p.Notification {
			b.notify(ctx, m.Chat.ID, b.text("notification_on"))
		} else {
			b.notify(ctx, m.Chat.ID, b.text("notification_off"))
		}
		return nil
	case "info", "ban", "unban":
		if id != admin || m.Chat.ID != admin {
			return b.say(ctx, m.Chat.ID, "not_an_admin")
		}
		var sender int64
		if m.ReplyToMessage != nil {
			var ok bool
			var err error
			sender, ok, err = b.replySender(m)
			if err != nil {
				return err
			}
			if !ok {
				return b.say(ctx, admin, "reply_to_message_no_data")
			}
		} else if command == "unban" && len(args) == 1 {
			var err error
			sender, err = strconv.ParseInt(args[0], 10, 64)
			if err != nil || sender <= 0 {
				return b.say(ctx, admin, "user_not_found")
			}
		} else if command == "unban" {
			return b.say(ctx, admin, "reply_or_enter_id")
		} else {
			return b.say(ctx, admin, "reply_to_no_message")
		}
		p, exists := b.store.LookupPreference(sender)
		if !exists {
			return b.say(ctx, admin, "user_not_found")
		}
		if command == "info" {
			return b.api.Send(ctx, admin, formatUser(b.text("info_data"), p.Name, sender), m.ReplyToMessage.MessageID)
		}
		p.Blocked = command == "ban"
		if err := b.store.SetPreferenceForUpdate(updateID, sender, p); err != nil {
			return err
		}
		key, userKey := "ban_user", "be_blocked_alert"
		if command == "unban" {
			key, userKey = "unban_user", "be_unbanned"
		}
		b.notify(ctx, admin, formatUser(b.text(key), p.Name, sender))
		b.notify(ctx, sender, b.text(userKey))
		return nil
	default:
		return b.say(ctx, m.Chat.ID, "nonexistent_command")
	}
}
