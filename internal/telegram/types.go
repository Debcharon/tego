package telegram

import (
	"encoding/json"
	"fmt"
	"strings"
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
