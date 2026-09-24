package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTelegramForwardAndCopy(t *testing.T) {
	methods := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		if payload["chat_id"] != float64(1) || payload["from_chat_id"] != float64(2) || payload["message_id"] != float64(3) {
			t.Errorf("unexpected payload: %v", payload)
		}
		methods = append(methods, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/forwardMessage" {
			w.Write([]byte(`{"ok":true,"result":{"message_id":77}}`))
			return
		}
		w.Write([]byte(`{"ok":true,"result":{"message_id":88}}`))
	}))
	defer server.Close()
	api := New("test")
	api.baseURL = server.URL + "/"
	message, err := api.Forward(context.Background(), 1, 2, 3)
	if err != nil || message.MessageID != 77 {
		t.Fatalf("forward: %v, %v", message, err)
	}
	if err := api.Copy(context.Background(), 1, 2, 3); err != nil {
		t.Fatal(err)
	}
	if len(methods) != 2 || methods[0] != "/forwardMessage" || methods[1] != "/copyMessage" {
		t.Fatal(methods)
	}
}

func TestSetCommandsScopes(t *testing.T) {
	var calls []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/setMyCommands" { t.Errorf("unexpected path %s", r.URL.Path) }
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil { t.Error(err) }
		calls = append(calls, body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{\"ok\":true,\"result\":true}"))
	}))
	defer server.Close()
	api := New("test"); api.baseURL = server.URL + "/"
	if err := api.SetCommands(context.Background(), 42, "en"); err != nil { t.Fatal(err) }
	if len(calls) != 2 { t.Fatalf("got %d calls", len(calls)) }
	visitorScope := calls[0]["scope"].(map[string]any)
	adminScope := calls[1]["scope"].(map[string]any)
	if visitorScope["type"] != "all_private_chats" || adminScope["type"] != "chat" || adminScope["chat_id"] != float64(42) { t.Fatalf("bad scopes: %+v", calls) }
	has := func(index int, command string) bool { for _, raw := range calls[index]["commands"].([]any) { if raw.(map[string]any)["command"] == command { return true } }; return false }
	if has(0, "ban") || has(0, "ping") || !has(0, "status") || !has(1, "ban") || has(1, "ping") { t.Fatalf("bad commands: %+v", calls) }
}
