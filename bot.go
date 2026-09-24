package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

var version = "v3.0"

//go:embed lang/*.json
var languages embed.FS

type BotAPI interface {
	send(context.Context, int64, string, int64) error
	forward(context.Context, int64, int64, int64) (Message, error)
	copy(context.Context, int64, int64, int64) error
	sendVerification(context.Context, int64, string, string, string) error
	clearVerification(context.Context, int64, string) error
}
type Bot struct {
	store    *Store
	api      BotAPI
	lang     map[string]string
	username string
	verify   *VerificationConfig
}

func loadLanguage(name string) (map[string]string, error) {
	if name != "en" && name != "zh_cn" && name != "zh_cn_moe" {
		return nil, fmt.Errorf("unsupported language %q", name)
	}
	data, err := languages.ReadFile("lang/" + name + ".json")
	if err != nil {
		return nil, err
	}
	var lang map[string]string
	if err := json.Unmarshal(data, &lang); err != nil {
		return nil, err
	}
	for _, key := range []string{"start", "notification_on", "notification_off", "info_data", "message_received_notification", "reply_to_no_message", "reply_to_message_no_data", "reply_type_not_supported", "reply_message_sent", "please_setup_first", "blocked_alert", "reply_message_failed", "be_blocked_alert", "ban_user", "unban_user", "nonexistent_command", "not_an_admin", "reply_or_enter_id", "user_not_found", "be_unbanned", "verification_required", "verification_button", "verification_success", "verification_failed"} {
		if lang[key] == "" {
			return nil, fmt.Errorf("language %s missing %s", name, key)
		}
	}
	return lang, nil
}
func (b *Bot) text(key string) string { return b.lang[key] }
func (b *Bot) say(ctx context.Context, chatID int64, key string) error {
	return b.api.send(ctx, chatID, b.text(key), 0)
}

func (b *Bot) handle(ctx context.Context, m *Message, updateID int64) error {
	if m == nil || m.From == nil || m.Chat.Type != "private" {
		return nil
	}
	if done, err := b.store.delivered(updateID); err != nil {
		return err
	} else if done {
		return nil
	}
	if err := b.store.initUser(*m.From); err != nil {
		return err
	}
	if b.verify != nil && m.From.ID != b.store.Config.Admin {
		if b.store.preference(m.From.ID).Blocked {
			return b.say(ctx, m.Chat.ID, "be_blocked_alert")
		}
		if m.WebAppData != nil {
			return b.acceptVerification(ctx, m, updateID)
		}
		verified, err := b.store.isVerified(m.From.ID)
		if err != nil {
			return err
		}
		if !verified {
			if command, _, ok := parseCommand(m.Text, b.username); ok && command == "start" {
				return b.promptVerification(ctx, m, true)
			}
			return b.promptVerification(ctx, m, false)
		}
	}
	if command, args, ok := parseCommand(m.Text, b.username); ok {
		return b.command(ctx, m, command, args, updateID)
	}
	return b.message(ctx, m, updateID)
}

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

func (b *Bot) notify(ctx context.Context, chatID int64, text string) {
	if err := b.api.send(ctx, chatID, text, 0); err != nil {
		log.Printf("optional notification to %d failed: %v", chatID, err)
	}
}

func (b *Bot) message(ctx context.Context, m *Message, updateID int64) error {
	admin := b.store.Config.Admin
	if admin == 0 {
		return b.say(ctx, m.Chat.ID, "please_setup_first")
	}
	if m.From.ID == admin {
		if m.ReplyToMessage == nil {
			return b.say(ctx, admin, "reply_to_no_message")
		}
		sender, ok, err := b.store.sender(m.ReplyToMessage.MessageID)
		if err != nil {
			return err
		}
		if !ok {
			return b.say(ctx, admin, "reply_to_message_no_data")
		}
		if err := b.api.copy(ctx, sender, m.Chat.ID, m.MessageID); err != nil {
			var apiErr *APIError
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
		if err := b.store.markDelivered(updateID); err != nil {
			return err
		}
		if b.store.preference(admin).Notification {
			p := b.store.preference(sender)
			b.notify(ctx, admin, formatUser(b.text("reply_message_sent"), p.Name, sender))
		}
		return nil
	}
	if b.store.preference(m.From.ID).Blocked {
		return b.say(ctx, m.Chat.ID, "be_blocked_alert")
	}
	forwarded, err := b.api.forward(ctx, admin, m.Chat.ID, m.MessageID)
	if err != nil {
		return err
	}
	if err := b.store.link(updateID, forwarded.MessageID, m.From.ID); err != nil {
		return err
	}
	if b.store.preference(m.From.ID).Notification {
		b.notify(ctx, m.Chat.ID, b.text("message_received_notification"))
	}
	return nil
}

func formatUser(pattern, name string, id int64) string {
	// Language files retain the old %s placeholders, but plain text avoids Markdown injection from user names.
	pattern = strings.Replace(pattern, "[Link](tg://user?id=%s)", "%s", 1)
	pattern = strings.Replace(pattern, "[链接](tg://user?id=%s)", "%s", 1)
	return fmt.Sprintf(pattern, name, strconv.FormatInt(id, 10))
}

func (b *Bot) replySender(m *Message) (int64, bool, error) {
	if m.ReplyToMessage == nil {
		return 0, false, nil
	}
	return b.store.sender(m.ReplyToMessage.MessageID)
}

func (b *Bot) command(ctx context.Context, m *Message, command string, args []string, updateID int64) error {
	id := m.From.ID
	admin := b.store.Config.Admin
	switch command {
	case "start":
		return b.say(ctx, m.Chat.ID, "start")
	case "help":
		return b.api.send(ctx, m.Chat.ID, "tego\n"+version+"\nhttps://github.com/Debcharon/tego", 0)
	case "ping":
		return b.api.send(ctx, m.Chat.ID, "Pong!", 0)
	case "notification":
		p := b.store.preference(id)
		p.Notification = !p.Notification
		if err := b.store.setPreferenceForUpdate(updateID, id, p); err != nil {
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
		p, exists := b.store.Preferences[strconv.FormatInt(sender, 10)]
		if !exists {
			return b.say(ctx, admin, "user_not_found")
		}
		if command == "info" {
			return b.api.send(ctx, admin, formatUser(b.text("info_data"), p.Name, sender), m.ReplyToMessage.MessageID)
		}
		p.Blocked = command == "ban"
		if err := b.store.setPreferenceForUpdate(updateID, sender, p); err != nil {
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

func (b *Bot) run(ctx context.Context, telegram *Telegram) error {
	backoff := time.Second
	for ctx.Err() == nil {
		updates, err := telegram.getUpdates(ctx, b.store.Offset)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Printf("poll failed: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		failed := false
		for _, update := range updates {
			if update.UpdateID < b.store.Offset {
				continue
			}
			if err := b.handle(ctx, update.Message, update.UpdateID); err != nil {
				log.Printf("update %d failed: %v", update.UpdateID, err)
				var apiErr *APIError
				if !errors.As(err, &apiErr) || (apiErr.Code != 400 && apiErr.Code != 403) {
					failed = true
					break
				}
			}
			if err := b.store.advanceOffset(update.UpdateID + 1); err != nil {
				return fmt.Errorf("save update offset: %w", err)
			}
		}
		if failed {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
	return nil
}
