package telegram

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

type Telegram struct {
	baseURL string
	client  *http.Client
}

func New(token string) *Telegram {
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
		return &APIError{Code: envelope.ErrorCode, Description: envelope.Description, RetryAfter: envelope.Parameters.RetryAfter}
	}
	if out != nil {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}
func (t *Telegram) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	var result []Update
	err := t.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 60, "allowed_updates": []string{"message", "callback_query"}}, &result)
	return result, err
}
func (t *Telegram) GetMe(ctx context.Context) (User, error) {
	var u User
	err := t.call(ctx, "getMe", map[string]any{}, &u)
	return u, err
}
func (t *Telegram) Send(ctx context.Context, chatID int64, text string, replyID int64) error {
	p := map[string]any{"chat_id": chatID, "text": text}
	if replyID != 0 {
		p["reply_parameters"] = map[string]any{"message_id": replyID}
	}
	return t.call(ctx, "sendMessage", p, nil)
}

type Button struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

func (t *Telegram) SendPanel(ctx context.Context, chatID int64, text string, buttons [][]Button) error {
	return t.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text,
		"reply_markup": map[string]any{"inline_keyboard": buttons}}, nil)
}
func (t *Telegram) EditPanel(ctx context.Context, chatID, messageID int64, text string, buttons [][]Button) error {
	return t.call(ctx, "editMessageText", map[string]any{"chat_id": chatID, "message_id": messageID, "text": text,
		"reply_markup": map[string]any{"inline_keyboard": buttons}}, nil)
}
func (t *Telegram) AnswerCallback(ctx context.Context, id, text string, alert bool) error {
	return t.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id, "text": text, "show_alert": alert}, nil)
}
func (t *Telegram) SendVerification(ctx context.Context, chatID int64, text, button, webURL string) error {
	return t.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text,
		"reply_markup": map[string]any{"keyboard": [][]any{{map[string]any{"text": button, "web_app": map[string]any{"url": webURL}}}}, "resize_keyboard": true, "one_time_keyboard": true}}, nil)
}
func (t *Telegram) ClearVerification(ctx context.Context, chatID int64, text string) error {
	return t.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text,
		"reply_markup": map[string]any{"remove_keyboard": true}}, nil)
}
func (t *Telegram) Forward(ctx context.Context, chatID, sourceID, messageID int64) (Message, error) {
	var result Message
	err := t.call(ctx, "forwardMessage", map[string]any{"chat_id": chatID, "from_chat_id": sourceID, "message_id": messageID}, &result)
	return result, err
}
func (t *Telegram) Copy(ctx context.Context, chatID, sourceID, messageID int64) error {
	return t.call(ctx, "copyMessage", map[string]any{"chat_id": chatID, "from_chat_id": sourceID, "message_id": messageID}, nil)
}
func (t *Telegram) SetCommands(ctx context.Context, adminID int64, language string) error {
	user := [][2]string{{"start", "Start the bot"}, {"help", "Show help"}, {"status", "Check bot status"}, {"notification", "Toggle confirmations"}}
	admin := [][2]string{{"start", "Open admin panel"}, {"help", "Show help"}, {"status", "Show bot status"}, {"notification", "Toggle confirmations"}, {"info", "Show sender"}, {"ban", "Ban sender"}, {"unban", "Unban sender"}, {"banlist", "List banned users"}, {"unverify", "Revoke verification"}}
	if language == "zh_cn" || language == "zh_cn_moe" {
		user = [][2]string{{"start", "开始使用"}, {"help", "查看帮助"}, {"status", "查看运行状态"}, {"notification", "切换消息确认提示"}}
		admin = [][2]string{{"start", "打开管理面板"}, {"help", "查看帮助"}, {"status", "查看运行状态"}, {"notification", "切换消息确认提示"}, {"info", "查看发送者"}, {"ban", "封禁发送者"}, {"unban", "解除封禁"}, {"banlist", "查看封禁名单"}, {"unverify", "撤销验证"}}
	}
	encode := func(entries [][2]string) []map[string]string {
		commands := make([]map[string]string, 0, len(entries))
		for _, entry := range entries {
			commands = append(commands, map[string]string{"command": entry[0], "description": entry[1]})
		}
		return commands
	}
	if err := t.call(ctx, "setMyCommands", map[string]any{"commands": encode(user), "scope": map[string]any{"type": "all_private_chats"}}, nil); err != nil {
		return err
	}
	return t.call(ctx, "setMyCommands", map[string]any{"commands": encode(admin), "scope": map[string]any{"type": "chat", "chat_id": adminID}}, nil)
}
