package bot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Debcharon/tego/internal/i18n"
	"github.com/Debcharon/tego/internal/store"
	"github.com/Debcharon/tego/internal/telegram"
)

type sentMessage struct {
	chatID  int64
	text    string
	replyID int64
}
type fakeAPI struct {
	sent         []sentMessage
	panelText    string
	panelButtons [][]telegram.Button
	editErr      error
	editAttempts int
	answerCount  int
	forwarded    int
	copied       int
	copyErr      error
	failSend     bool
}

func (f *fakeAPI) Send(_ context.Context, id int64, text string, reply int64) error {
	f.sent = append(f.sent, sentMessage{id, text, reply})
	if f.failSend {
		return errors.New("notification unavailable")
	}
	return nil
}
func (f *fakeAPI) SendVerification(ctx context.Context, id int64, text, button, url string) error {
	return f.Send(ctx, id, text, 0)
}
func (f *fakeAPI) ClearVerification(ctx context.Context, id int64, text string) error {
	return f.Send(ctx, id, text, 0)
}
func (f *fakeAPI) Forward(_ context.Context, _, _, _ int64) (telegram.Message, error) {
	f.forwarded++
	return telegram.Message{MessageID: 77}, nil
}
func (f *fakeAPI) Copy(_ context.Context, _, _, _ int64) error { f.copied++; return f.copyErr }

func testBot(t *testing.T) (*Bot, *fakeAPI) {
	t.Helper()
	dir := t.TempDir()

	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	l, err := i18n.Load("en")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeAPI{}
	return &Bot{adminID: 1, store: s, api: f, lang: l, username: "testbot"}, f
}

func privateMessage(id, messageID int64, text string) *telegram.Message {
	return &telegram.Message{MessageID: messageID, From: &telegram.User{ID: id, FirstName: "User"}, Chat: telegram.Chat{ID: id, Type: "private"}, Text: text}
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
	if sender, ok, err := b.store.Sender(77); err != nil || !ok || sender != 2 {
		t.Fatal("message mapping not saved")
	}
	adminReply := privateMessage(1, 11, "answer")
	adminReply.ReplyToMessage = &telegram.Message{MessageID: 77}
	if err := b.handle(ctx, adminReply, 0); err != nil {
		t.Fatal(err)
	}
	if api.copied != 1 {
		t.Fatal("admin answer not copied")
	}
	ban := privateMessage(1, 12, "/ban")
	ban.ReplyToMessage = &telegram.Message{MessageID: 77}
	if err := b.handle(ctx, ban, 0); err != nil {
		t.Fatal(err)
	}
	if !b.store.Preference(2).Blocked {
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
	if b.store.Preference(2).Blocked {
		t.Fatal("sender not unbanned")
	}
	reloaded, err := store.Open(b.store.Directory())
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	if reloaded.Preference(2).Blocked {
		t.Fatal("unban not persisted")
	}
}

func TestCommandsCannotClaimAdmin(t *testing.T) {
	b, api := testBot(t)
	ctx := context.Background()
	if err := b.handle(ctx, privateMessage(2, 1, "/setadmin@testbot"), 0); err != nil {
		t.Fatal(err)
	}
	if b.adminID != 1 {
		t.Fatal("admin changed")
	}
	if err := b.handle(ctx, privateMessage(3, 2, "/notification"), 0); err != nil {
		t.Fatal(err)
	}
	if !b.store.Preference(3).Notification {
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
	if err := b.store.SetPreference(2, store.Preference{Name: "User", Notification: true}); err != nil {
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
	reloaded, err := store.Open(b.store.Directory())
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	again := &Bot{adminID: 1, store: reloaded, api: api, lang: b.lang, username: b.username}
	if err := again.handle(ctx, message, 100); err != nil {
		t.Fatal(err)
	}
	if api.forwarded != 1 {
		t.Fatalf("forwarded after restart %d times", api.forwarded)
	}
	if err := reloaded.AdvanceOffset(101); err != nil {
		t.Fatal(err)
	}
	if reloaded.Offset != 101 {
		t.Fatalf("offset = %d", reloaded.Offset)
	}
	if done, err := reloaded.Delivered(100); err != nil || done {
		t.Fatalf("marker should be pruned: %v, %v", done, err)
	}
}

func TestAdminCopyAndToggleAreNotRepeated(t *testing.T) {
	b, api := testBot(t)
	ctx := context.Background()
	if err := b.store.Link(90, 77, 2); err != nil {
		t.Fatal(err)
	}
	if err := b.store.SetPreference(1, store.Preference{Name: "Admin", Notification: true}); err != nil {
		t.Fatal(err)
	}
	api.failSend = true
	reply := privateMessage(1, 20, "answer")
	reply.ReplyToMessage = &telegram.Message{MessageID: 77}
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
	if !b.store.Preference(2).Notification {
		t.Fatal("toggle applied twice")
	}
}

func TestSQLiteErrorIsReturned(t *testing.T) {
	b, _ := testBot(t)
	if err := b.store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.store.Delivered(123); err == nil {
		t.Fatal("closed database read succeeded")
	}
	if err := b.store.AdvanceOffset(124); err == nil {
		t.Fatal("closed database write succeeded")
	}
}

func TestBanByIDAndCommandHelp(t *testing.T) {
	b, api := testBot(t)
	ctx := context.Background()
	if err := b.handle(ctx, privateMessage(2, 10, "hello"), 0); err != nil {
		t.Fatal(err)
	}
	if err := b.handle(ctx, privateMessage(3, 11, "/ban 2"), 0); err != nil {
		t.Fatal(err)
	}
	if b.store.Preference(2).Blocked {
		t.Fatal("visitor banned another user")
	}
	if err := b.handle(ctx, privateMessage(1, 12, "/ban 2"), 0); err != nil {
		t.Fatal(err)
	}
	if !b.store.Preference(2).Blocked {
		t.Fatal("admin ban by ID failed")
	}
	if err := b.handle(ctx, privateMessage(1, 13, "/help"), 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(api.sent[len(api.sent)-1].text, "/ban <user ID>") {
		t.Fatal("admin help missing ban usage")
	}
	if err := b.handle(ctx, privateMessage(3, 14, "/help"), 0); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(api.sent[len(api.sent)-1].text, "/ban") {
		t.Fatal("visitor help exposes admin command")
	}
}
func (f *fakeAPI) SendPanel(_ context.Context, _ int64, text string, buttons [][]telegram.Button) error {
	f.panelText, f.panelButtons = text, buttons
	return nil
}
func (f *fakeAPI) EditPanel(_ context.Context, _, _ int64, text string, buttons [][]telegram.Button) error {
	f.editAttempts++
	if f.editErr != nil {
		return f.editErr
	}
	f.panelText, f.panelButtons = text, buttons
	return nil
}
func (f *fakeAPI) AnswerCallback(_ context.Context, _ string, _ string, _ bool) error {
	f.answerCount++
	return nil
}

func TestStatusReplacesPing(t *testing.T) {
	b, api := testBot(t)
	ctx := context.Background()
	if err := b.handle(ctx, privateMessage(1, 1, "/status"), 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(api.sent[len(api.sent)-1].text, "Verification: disabled") {
		t.Fatal("admin status missing verification mode")
	}
	if err := b.handle(ctx, privateMessage(2, 2, "/status"), 0); err != nil {
		t.Fatal(err)
	}
	if api.sent[len(api.sent)-1].text != b.text("status_user") {
		t.Fatal("visitor status leaked admin details")
	}
	if err := b.handle(ctx, privateMessage(2, 3, "/ping"), 0); err != nil {
		t.Fatal(err)
	}
	if api.sent[len(api.sent)-1].text != b.text("nonexistent_command") {
		t.Fatal("ping still active")
	}
}
