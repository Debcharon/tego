package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

func (u User) FullName() string { return strings.TrimSpace(u.FirstName + " " + u.LastName) }

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}
type WebAppData struct {
	Data string `json:"data"`
}
type Message struct {
	MessageID      int64       `json:"message_id"`
	From           *User       `json:"from"`
	Chat           Chat        `json:"chat"`
	Text           string      `json:"text"`
	ReplyToMessage *Message    `json:"reply_to_message"`
	WebAppData     *WebAppData `json:"web_app_data"`
}
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}
type apiEnvelope struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Result      json.RawMessage `json:"result"`
}
type APIError struct {
	Code        int
	Description string
}

func (e *APIError) Error() string { return fmt.Sprintf("Telegram API %d: %s", e.Code, e.Description) }

type Telegram struct {
	baseURL string
	client  *http.Client
}

func newTelegram(token string) *Telegram {
	return &Telegram{baseURL: "https://api.telegram.org/bot" + token + "/", client: &http.Client{Timeout: 75 * time.Second}}
}
func (t *Telegram) call(ctx context.Context, method string, params any, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %s", method, strings.ReplaceAll(err.Error(), t.baseURL, "[Telegram API]/"))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode %s: %w", method, err)
	}
	if !envelope.OK {
		return &APIError{Code: envelope.ErrorCode, Description: envelope.Description}
	}
	if out != nil {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}
func (t *Telegram) getUpdates(ctx context.Context, offset int64) ([]Update, error) {
	var result []Update
	err := t.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 60, "allowed_updates": []string{"message"}}, &result)
	return result, err
}
func (t *Telegram) getMe(ctx context.Context) (User, error) {
	var u User
	err := t.call(ctx, "getMe", map[string]any{}, &u)
	return u, err
}
func (t *Telegram) send(ctx context.Context, chatID int64, text string, replyID int64) error {
	p := map[string]any{"chat_id": chatID, "text": text}
	if replyID != 0 {
		p["reply_parameters"] = map[string]any{"message_id": replyID}
	}
	return t.call(ctx, "sendMessage", p, nil)
}
func (t *Telegram) sendVerification(ctx context.Context, chatID int64, text, button, webURL string) error {
	return t.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text,
		"reply_markup": map[string]any{"keyboard": [][]any{{map[string]any{"text": button, "web_app": map[string]any{"url": webURL}}}}, "resize_keyboard": true, "one_time_keyboard": true}}, nil)
}
func (t *Telegram) clearVerification(ctx context.Context, chatID int64, text string) error {
	return t.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text,
		"reply_markup": map[string]any{"remove_keyboard": true}}, nil)
}
func (t *Telegram) forward(ctx context.Context, chatID, sourceID, messageID int64) (Message, error) {
	var result Message
	err := t.call(ctx, "forwardMessage", map[string]any{"chat_id": chatID, "from_chat_id": sourceID, "message_id": messageID}, &result)
	return result, err
}
func (t *Telegram) copy(ctx context.Context, chatID, sourceID, messageID int64) error {
	return t.call(ctx, "copyMessage", map[string]any{"chat_id": chatID, "from_chat_id": sourceID, "message_id": messageID}, nil)
}
func (t *Telegram) setCommands(ctx context.Context) error {
	commands := []map[string]string{}
	for _, entry := range [][2]string{{"start", "Start the bot"}, {"help", "Show help"}, {"ping", "Check bot status"}, {"notification", "Toggle notifications"}, {"info", "Show sender"}, {"ban", "Ban sender"}, {"unban", "Unban sender"}} {
		commands = append(commands, map[string]string{"command": entry[0], "description": entry[1]})
	}
	return t.call(ctx, "setMyCommands", map[string]any{"commands": commands}, nil)
}
