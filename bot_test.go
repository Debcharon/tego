package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sentMessage struct {
	chatID  int64
	text    string
	replyID int64
}
type fakeAPI struct {
	sent      []sentMessage
	forwarded int
	copied    int
	copyErr   error
	failSend  bool
}

func (f *fakeAPI) send(_ context.Context, id int64, text string, reply int64) error {
	f.sent = append(f.sent, sentMessage{id, text, reply})
	if f.failSend {
		return errors.New("notification unavailable")
	}
	return nil
}
func (f *fakeAPI) sendVerification(ctx context.Context, id int64, text, button, url string) error {
	return f.send(ctx, id, text, 0)
}
func (f *fakeAPI) clearVerification(ctx context.Context, id int64, text string) error {
	return f.send(ctx, id, text, 0)
}
func (f *fakeAPI) forward(_ context.Context, _, _, _ int64) (Message, error) {
	f.forwarded++
	return Message{MessageID: 77}, nil
}
func (f *fakeAPI) copy(_ context.Context, _, _, _ int64) error { f.copied++; return f.copyErr }

func testBot(t *testing.T) (*Bot, *fakeAPI) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"admin":1,"lang":"en"}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := loadStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	l, err := loadLanguage("en")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeAPI{}
	return &Bot{store: s, api: f, lang: l, username: "testbot"}, f
}

func privateMessage(id, messageID int64, text string) *Message {
	return &Message{MessageID: messageID, From: &User{ID: id, FirstName: "User"}, Chat: Chat{ID: id, Type: "private"}, Text: text}
}

func TestRelayReplyAndBan(t *testing.T) {
	b, api := testBot(t)
	ctx := context.Background()
	if err := b.handle(ctx, privateMessage(2, 10, "hello"), 0); err != nil {
		t.Fatal(err)
	}
	if api.forwarded != 1 {
		t.Fatal("message not forwarded")
	}
	if sender, ok, err := b.store.sender(77); err != nil || !ok || sender != 2 {
		t.Fatal("message mapping not saved")
	}
	adminReply := privateMessage(1, 11, "answer")
	adminReply.ReplyToMessage = &Message{MessageID: 77}
	if err := b.handle(ctx, adminReply, 0); err != nil {
		t.Fatal(err)
	}
	if api.copied != 1 {
		t.Fatal("admin answer not copied")
	}
	ban := privateMessage(1, 12, "/ban")
	ban.ReplyToMessage = &Message{MessageID: 77}
	if err := b.handle(ctx, ban, 0); err != nil {
		t.Fatal(err)
	}
	if !b.store.preference(2).Blocked {
		t.Fatal("sender not banned")
	}
	if err := b.handle(ctx, privateMessage(2, 13, "again"), 0); err != nil {
		t.Fatal(err)
	}
	if api.forwarded != 1 {
		t.Fatal("banned user forwarded")
	}
	if err := b.handle(ctx, privateMessage(1, 14, "/unban 2"), 0); err != nil {
		t.Fatal(err)
	}
	if b.store.preference(2).Blocked {
		t.Fatal("sender not unbanned")
	}
	reloaded, err := loadStore(b.store.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	if reloaded.preference(2).Blocked {
		t.Fatal("unban not persisted")
	}
}

func TestCommandsCannotClaimAdmin(t *testing.T) {
	b, api := testBot(t)
	ctx := context.Background()
	if err := b.handle(ctx, privateMessage(2, 1, "/setadmin@testbot"), 0); err != nil {
		t.Fatal(err)
	}
	if b.store.Config.Admin != 1 {
		t.Fatal("admin changed")
	}
	if err := b.handle(ctx, privateMessage(3, 2, "/notification"), 0); err != nil {
		t.Fatal(err)
	}
	if !b.store.preference(3).Notification {
		t.Fatal("notification not enabled")
	}
	if len(api.sent) != 2 {
		t.Fatalf("want two responses, got %d", len(api.sent))
	}
}

func TestUserNameIsPlainText(t *testing.T) {
	result := formatUser("Sender: %s [Link](tg://user?id=%s)", "[x](https://example.com)", 42)
	if !strings.Contains(result, "[x](https://example.com) 42") {
		t.Fatal(result)
	}
}

func TestDeliverySurvivesNotificationFailureAndRestart(t *testing.T) {
	b, api := testBot(t)
	ctx := context.Background()
	if err := b.store.setPreference(2, Preference{Name: "User", Notification: true}); err != nil {
		t.Fatal(err)
	}
	api.failSend = true
	message := privateMessage(2, 10, "hello")
	if err := b.handle(ctx, message, 100); err != nil {
		t.Fatal(err)
	}
	if err := b.handle(ctx, message, 100); err != nil {
		t.Fatal(err)
	}
	if api.forwarded != 1 {
		t.Fatalf("forwarded %d times", api.forwarded)
	}
	reloaded, err := loadStore(b.store.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	again := &Bot{store: reloaded, api: api, lang: b.lang, username: b.username}
	if err := again.handle(ctx, message, 100); err != nil {
		t.Fatal(err)
	}
	if api.forwarded != 1 {
		t.Fatalf("forwarded after restart %d times", api.forwarded)
	}
	if err := reloaded.advanceOffset(101); err != nil {
		t.Fatal(err)
	}
	if reloaded.Offset != 101 {
		t.Fatalf("offset = %d", reloaded.Offset)
	}
	if done, err := reloaded.delivered(100); err != nil || done {
		t.Fatalf("marker should be pruned: %v, %v", done, err)
	}
}

func TestAdminCopyAndToggleAreNotRepeated(t *testing.T) {
	b, api := testBot(t)
	ctx := context.Background()
	if err := b.store.link(90, 77, 2); err != nil {
		t.Fatal(err)
	}
	if err := b.store.setPreference(1, Preference{Name: "Admin", Notification: true}); err != nil {
		t.Fatal(err)
	}
	api.failSend = true
	reply := privateMessage(1, 20, "answer")
	reply.ReplyToMessage = &Message{MessageID: 77}
	if err := b.handle(ctx, reply, 91); err != nil {
		t.Fatal(err)
	}
	if err := b.handle(ctx, reply, 91); err != nil {
		t.Fatal(err)
	}
	if api.copied != 1 {
		t.Fatalf("copied %d times", api.copied)
	}
	toggle := privateMessage(2, 21, "/notification")
	if err := b.handle(ctx, toggle, 92); err != nil {
		t.Fatal(err)
	}
	if err := b.handle(ctx, toggle, 92); err != nil {
		t.Fatal(err)
	}
	if !b.store.preference(2).Notification {
		t.Fatal("toggle applied twice")
	}
}

func TestLanguagesHaveRequiredMessages(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"en", "zh_cn", "zh_cn_moe"} {
		if _, err := loadLanguage(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestInvalidAdminDoesNotCreateDatabase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"admin":0,"lang":"en"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadStore(dir); err == nil {
		t.Fatal("invalid admin accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "bot.db")); !os.IsNotExist(err) {
		t.Fatalf("database created: %v", err)
	}
}

func TestSQLiteErrorIsReturned(t *testing.T) {
	b, _ := testBot(t)
	if err := b.store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.store.delivered(123); err == nil {
		t.Fatal("closed database read succeeded")
	}
	if err := b.store.advanceOffset(124); err == nil {
		t.Fatal("closed database write succeeded")
	}
}
