package bot

import (
	"errors"
	"testing"
	"time"

	"github.com/Debcharon/tego/internal/telegram"
)

func TestRetryDelayUsesTelegramLimit(t *testing.T) {
	if got := retryDelay(&telegram.APIError{Code: 429, RetryAfter: 17}, time.Second); got != 17*time.Second {
		t.Fatalf("retry delay = %s", got)
	}
	if got := retryDelay(errors.New("network"), 3*time.Second); got != 3*time.Second {
		t.Fatalf("fallback delay = %s", got)
	}
}
