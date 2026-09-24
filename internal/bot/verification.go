package bot

import (
	"context"
	"log"
	"time"

	"github.com/Debcharon/tego/internal/telegram"
)

const promptCooldown = 30 * time.Second

func (b *Bot) promptVerification(ctx context.Context, m *telegram.Message, force bool) error {
	now := time.Now().Unix()
	nonce, expiresAt, promptedAt, err := b.store.Challenge(m.From.ID, now)
	if err != nil {
		return err
	}
	if !force && promptedAt > 0 && now-promptedAt < int64(promptCooldown.Seconds()) {
		return nil
	}
	if err := b.api.SendVerification(ctx, m.Chat.ID, b.text("verification_required"),
		b.text("verification_button"), b.verify.ChallengeURL(m.From.ID, expiresAt, nonce)); err != nil {
		return err
	}
	return b.store.MarkChallengePrompted(m.From.ID, nonce, now)
}

func (b *Bot) acceptVerification(ctx context.Context, m *telegram.Message, updateID int64) error {
	now := time.Now().Unix()
	nonce, valid := b.verify.ParseProof(m.WebAppData.Data, m.From.ID, now)
	if !valid {
		return b.say(ctx, m.Chat.ID, "verification_failed")
	}
	accepted, err := b.store.ConsumeProof(updateID, m.From.ID, nonce, now)
	if err != nil {
		return err
	}
	if !accepted {
		return b.say(ctx, m.Chat.ID, "verification_failed")
	}
	if err := b.api.ClearVerification(ctx, m.Chat.ID, b.text("verification_success")); err != nil {
		log.Printf("verification confirmation to %d failed: %v", m.Chat.ID, err)
		return nil
	}
	return nil
}
