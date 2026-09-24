package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Debcharon/tego/internal/telegram"
)

func (b *Bot) Run(ctx context.Context, client *telegram.Telegram) error {
	backoff := time.Second
	for ctx.Err() == nil {
		updates, err := client.GetUpdates(ctx, b.store.Offset)
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
				var apiErr *telegram.APIError
				if !errors.As(err, &apiErr) || (apiErr.Code != 400 && apiErr.Code != 403) {
					failed = true
					break
				}
			}
			if err := b.store.AdvanceOffset(update.UpdateID + 1); err != nil {
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
