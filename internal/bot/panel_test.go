package bot

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Debcharon/tego/internal/store"
	"github.com/Debcharon/tego/internal/telegram"
)

func panelUpdate(id int64, actor int64, data string) telegram.Update {
	return telegram.Update{UpdateID: id, CallbackQuery: &telegram.CallbackQuery{
		ID: "callback", From: telegram.User{ID: actor}, Data: data,
		Message: &telegram.Message{MessageID: 50, Chat: telegram.Chat{ID: 1, Type: "private"}},
	}}
}

func TestAdminStartPanelAndBanlist(t *testing.T) {
	b, api := testBot(t)
	ctx := context.Background()
	if err := b.handle(ctx, privateMessage(1, 1, "/start"), 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(api.panelText, "Admin panel") || len(api.panelButtons) == 0 {
		t.Fatal("admin panel missing")
	}
	if err := b.handle(ctx, privateMessage(2, 2, "/start"), 2); err != nil {
		t.Fatal(err)
	}
	if len(api.sent) == 0 || api.sent[len(api.sent)-1].text != b.text("start") {
		t.Fatal("visitor start changed")
	}
	if err := b.handle(ctx, privateMessage(2, 3, "hello"), 3); err != nil {
		t.Fatal(err)
	}
	if err := b.handle(ctx, privateMessage(1, 4, "/ban 2"), 4); err != nil {
		t.Fatal(err)
	}
	if err := b.handle(ctx, privateMessage(1, 5, "/banlist"), 5); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(api.panelText, "Banned users") || !strings.Contains(api.panelButtons[0][0].Text, "2") {
		t.Fatal("banlist missing user")
	}
	before := api.panelText
	if err := b.handle(ctx, privateMessage(3, 6, "/banlist"), 6); err != nil {
		t.Fatal(err)
	}
	if api.panelText != before {
		t.Fatal("visitor opened banlist")
	}
}

func TestPanelCallbackAuthorizationAndConfirmation(t *testing.T) {
	b, api := testBot(t)
	ctx := context.Background()
	if err := b.handle(ctx, privateMessage(2, 1, "hello"), 1); err != nil {
		t.Fatal(err)
	}
	if err := b.handleUpdate(ctx, panelUpdate(10, 2, "confirm:ban:2")); err != nil {
		t.Fatal(err)
	}
	if b.store.Preference(2).Blocked {
		t.Fatal("visitor changed ban")
	}
	if err := b.handleUpdate(ctx, panelUpdate(11, 1, "confirm:ban:2")); err != nil {
		t.Fatal(err)
	}
	if b.store.Preference(2).Blocked || !strings.Contains(api.panelText, "Confirm Ban") {
		t.Fatal("confirmation skipped")
	}
	if err := b.handleUpdate(ctx, panelUpdate(12, 1, "do:ban:2")); err != nil {
		t.Fatal(err)
	}
	if !b.store.Preference(2).Blocked {
		t.Fatal("panel ban failed")
	}
	if err := b.handleUpdate(ctx, panelUpdate(12, 1, "do:unban:2")); err != nil {
		t.Fatal(err)
	}
	if !b.store.Preference(2).Blocked {
		t.Fatal("replayed update changed ban")
	}
	if api.answerCount < 3 {
		t.Fatal("callback not answered")
	}
}

func TestUnverifyCommand(t *testing.T) {
	b, api := testBot(t)
	ctx := context.Background()
	if err := b.store.InitUser(2, "User"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	nonce, _, _, err := b.store.Challenge(2, now)
	if err != nil {
		t.Fatal(err)
	}
	if accepted, err := b.store.ConsumeProof(0, 2, nonce, now); err != nil || !accepted {
		t.Fatalf("verify: %v %v", accepted, err)
	}
	if err := b.handle(ctx, privateMessage(3, 1, "/unverify 2"), 1); err != nil {
		t.Fatal(err)
	}
	if verified, _ := b.store.IsVerified(2); !verified {
		t.Fatal("visitor revoked verification")
	}
	if err := b.handle(ctx, privateMessage(1, 2, "/unverify 2"), 2); err != nil {
		t.Fatal(err)
	}
	if verified, _ := b.store.IsVerified(2); verified {
		t.Fatal("admin did not revoke verification")
	}
	if !strings.Contains(api.sent[len(api.sent)-1].text, "revoked") {
		t.Fatal("missing confirmation")
	}
}

func TestPanelRetriesEditWithoutRepeatingActions(t *testing.T) {
	for _, action := range []string{"ban", "unverify", "toggle"} {
		t.Run(action, func(t *testing.T) {
			b, api := testBot(t)
			if err := b.store.InitUser(2, "User"); err != nil {
				t.Fatal(err)
			}
			if action == "unverify" {
				now := time.Now().Unix()
				nonce, _, _, err := b.store.Challenge(2, now)
				if err != nil {
					t.Fatal(err)
				}
				if accepted, err := b.store.ConsumeProof(0, 2, nonce, now); err != nil || !accepted {
					t.Fatalf("verify: %v %v", accepted, err)
				}
			}
			data := "do:" + action + ":2"
			if action == "toggle" {
				data = "toggle"
			}
			update := panelUpdate(100, 1, data)
			api.panelText = "old panel"
			api.editErr = &telegram.APIError{Code: 429, RetryAfter: 1}
			if err := b.handleUpdate(context.Background(), update); err == nil {
				t.Fatal("edit error not returned")
			}
			if api.panelText != "old panel" {
				t.Fatal("failed edit updated panel")
			}
			if done, err := b.store.Delivered(100); err != nil || !done {
				t.Fatalf("action not committed: %v %v", done, err)
			}
			messagesSent := len(api.sent)
			api.editErr = nil
			if err := b.handleUpdate(context.Background(), update); err != nil {
				t.Fatal(err)
			}
			if api.editAttempts != 2 || api.answerCount != 1 || len(api.sent) != messagesSent {
				t.Fatalf("action repeated: edits=%d answers=%d messages=%d", api.editAttempts, api.answerCount, len(api.sent))
			}
			switch action {
			case "ban":
				if !b.store.Preference(2).Blocked || !strings.Contains(api.panelText, "Banned: yes") {
					t.Fatal("ban panel not refreshed")
				}
			case "unverify":
				if verified, err := b.store.IsVerified(2); err != nil || verified || !strings.Contains(api.panelText, "Verified: no") {
					t.Fatal("verification panel not refreshed")
				}
			case "toggle":
				if !b.store.Preference(1).Notification || !strings.Contains(api.panelText, b.text("notification_on")) {
					t.Fatal("settings panel not refreshed")
				}
			}
		})
	}
}

func TestPanelListNavigation(t *testing.T) {
	for _, list := range []string{"users", "bans", "verified"} {
		t.Run(list, func(t *testing.T) {
			b, api := testBot(t)
			ctx := context.Background()
			for id := int64(2); id <= 20; id++ {
				if err := b.store.SetPreference(id, store.Preference{Name: "User", Blocked: true}); err != nil {
					t.Fatal(err)
				}
				now := time.Now().Unix()
				nonce, _, _, err := b.store.Challenge(id, now)
				if err != nil {
					t.Fatal(err)
				}
				if ok, err := b.store.ConsumeProof(0, id, nonce, now); err != nil || !ok {
					t.Fatalf("verify: %v %v", ok, err)
				}
			}
			origin := list + ":2"
			if err := b.handleUpdate(ctx, panelUpdate(100, 1, origin)); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(api.panelText, "Page 3/3 · 19 users") || !strings.HasPrefix(api.panelButtons[0][0].Text, "🚫 ✓ ") {
				t.Fatalf("list summary or markers missing: %s %+v", api.panelText, api.panelButtons)
			}
			detail := api.panelButtons[0][0].Data
			if err := b.handleUpdate(ctx, panelUpdate(101, 1, detail)); err != nil {
				t.Fatal(err)
			}
			assertPanelBack(t, api, origin)
			confirmation := api.panelButtons[0][0].Data
			if err := b.handleUpdate(ctx, panelUpdate(102, 1, confirmation)); err != nil {
				t.Fatal(err)
			}
			cancel := api.panelButtons[0][1].Data
			if cancel != detail {
				t.Fatalf("cancel lost origin: %s", cancel)
			}
			action := api.panelButtons[0][0].Data
			api.editErr = &telegram.APIError{Code: 429, RetryAfter: 1}
			update := panelUpdate(103, 1, action)
			if err := b.handleUpdate(ctx, update); err == nil {
				t.Fatal("expected edit failure")
			}
			api.editErr = nil
			if err := b.handleUpdate(ctx, update); err != nil {
				t.Fatal(err)
			}
			assertPanelBack(t, api, origin)
			if err := b.handleUpdate(ctx, panelUpdate(104, 1, origin)); err != nil {
				t.Fatal(err)
			}
			total := 19
			if list == "bans" {
				total--
			}
			if !strings.Contains(api.panelText, fmt.Sprintf("Page 3/3 · %d users", total)) {
				t.Fatalf("wrong return page: %s", api.panelText)
			}
		})
	}
}

func assertPanelBack(t *testing.T, api *fakeAPI, origin string) {
	t.Helper()
	buttons := api.panelButtons
	if got := buttons[len(buttons)-1][0].Data; got != origin {
		t.Fatalf("back route = %s, want %s", got, origin)
	}
}

func TestPanelClampsEmptyLastPage(t *testing.T) {
	b, api := testBot(t)
	for id := int64(2); id <= 10; id++ {
		if err := b.store.SetPreference(id, store.Preference{Name: "User", Blocked: true}); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if err := b.handleUpdate(ctx, panelUpdate(1, 1, "bans:1")); err != nil {
		t.Fatal(err)
	}
	if err := b.handleUpdate(ctx, panelUpdate(2, 1, api.panelButtons[0][0].Data)); err != nil {
		t.Fatal(err)
	}
	if err := b.handleUpdate(ctx, panelUpdate(3, 1, api.panelButtons[0][0].Data)); err != nil {
		t.Fatal(err)
	}
	if err := b.handleUpdate(ctx, panelUpdate(4, 1, api.panelButtons[0][0].Data)); err != nil {
		t.Fatal(err)
	}
	assertPanelBack(t, api, "bans:1")
	if err := b.handleUpdate(ctx, panelUpdate(5, 1, "bans:1")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(api.panelText, "Page 1/1 · 8 users") || api.panelButtons[0][0].Data == "" {
		t.Fatalf("empty final page not clamped: %s", api.panelText)
	}
}

func TestPanelEmptyListsAndInvalidOrigins(t *testing.T) {
	b, api := testBot(t)
	for _, route := range []string{"users:0", "bans:0", "verified:0"} {
		if err := b.handleUpdate(context.Background(), panelUpdate(1, 1, route)); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(api.panelText, "Page 1/1 · 0 users") || !strings.Contains(api.panelText, b.text("panel_empty")) {
			t.Fatalf("empty summary: %s", api.panelText)
		}
	}
	for _, route := range []string{"user:2|bans:-1", "user:2|home", "do:ban:2|users:0|bans:0", "users:0:extra"} {
		if err := b.handleUpdate(context.Background(), panelUpdate(2, 1, route)); err != nil {
			t.Fatal(err)
		}
		if api.panelText != b.text("panel_invalid") {
			t.Fatalf("invalid route accepted: %s", route)
		}
	}
}
