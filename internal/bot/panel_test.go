package bot

import (
	"context"
	"strings"
	"testing"
	"time"

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
