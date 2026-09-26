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
	nextCleanup := time.Time{}
	for ctx.Err() == nil {
		if !time.Now().Before(nextCleanup) {
			challenges, messages, err := b.store.Cleanup(time.Now())
			if err != nil {
				return fmt.Errorf("clean database: %w", err)
			}
			if challenges+messages > 0 {
				log.Printf("cleaned %d expired challenges and %d old message mappings", challenges, messages)
			}
			nextCleanup = time.Now().Add(24 * time.Hour)
		}
		updates, err := client.GetUpdates(ctx, b.store.Offset)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Printf("poll failed: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(retryDelay(err, backoff)):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		failed := false
		var failure error
		for _, update := range updates {
			if update.UpdateID < b.store.Offset {
				continue
			}
			if err := b.handleUpdate(ctx, update); err != nil {
				log.Printf("update %d failed: %v", update.UpdateID, err)
				var apiErr *telegram.APIError
				if !errors.As(err, &apiErr) || (apiErr.Code != 400 && apiErr.Code != 403) {
					failed = true
					failure = err
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
			case <-time.After(retryDelay(failure, backoff)):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
	return nil
}

func retryDelay(err error, fallback time.Duration) time.Duration {
	var apiErr *telegram.APIError
	if errors.As(err, &apiErr) && apiErr.Code == 429 && apiErr.RetryAfter > 0 {
		return time.Duration(apiErr.RetryAfter) * time.Second
	}
	return fallback
}
