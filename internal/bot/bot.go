package bot

import (
	"context"

	"github.com/Debcharon/tego/internal/store"
	"github.com/Debcharon/tego/internal/telegram"
	"github.com/Debcharon/tego/internal/verification"
)

type BotAPI interface {
	Send(context.Context, int64, string, int64) error
	Forward(context.Context, int64, int64, int64) (telegram.Message, error)
	Copy(context.Context, int64, int64, int64) error
	SendVerification(context.Context, int64, string, string, string) error
	ClearVerification(context.Context, int64, string) error
}
type Bot struct {
	adminID  int64
	version  string
	store    *store.Store
	api      BotAPI
	lang     map[string]string
	username string
	verify   *verification.Config
}

func (b *Bot) text(key string) string { return b.lang[key] }
func (b *Bot) say(ctx context.Context, chatID int64, key string) error {
	return b.api.Send(ctx, chatID, b.text(key), 0)
}

func (b *Bot) handle(ctx context.Context, m *telegram.Message, updateID int64) error {
	if m == nil || m.From == nil || m.Chat.Type != "private" {
		return nil
	}
	if done, err := b.store.Delivered(updateID); err != nil {
		return err
	} else if done {
		return nil
	}
	if err := b.store.InitUser(m.From.ID, m.From.FullName()); err != nil {
		return err
	}
	if b.verify != nil && m.From.ID != b.adminID {
		if b.store.Preference(m.From.ID).Blocked {
			return b.say(ctx, m.Chat.ID, "be_blocked_alert")
		}
		if m.WebAppData != nil {
			return b.acceptVerification(ctx, m, updateID)
		}
		verified, err := b.store.IsVerified(m.From.ID)
		if err != nil {
			return err
		}
		if !verified {
			if command, args, ok := parseCommand(m.Text, b.username); ok {
				if command == "start" {
					return b.promptVerification(ctx, m, true)
				}
				if command == "help" || command == "status" {
					return b.command(ctx, m, command, args, updateID)
				}
			}
			return b.promptVerification(ctx, m, false)
		}
	}
	if command, args, ok := parseCommand(m.Text, b.username); ok {
		return b.command(ctx, m, command, args, updateID)
	}
	return b.message(ctx, m, updateID)
}

func New(s *store.Store, api BotAPI, language map[string]string, username string, adminID int64, verify *verification.Config, version string) *Bot {
	return &Bot{store: s, api: api, lang: language, username: username, adminID: adminID, verify: verify, version: version}
}
