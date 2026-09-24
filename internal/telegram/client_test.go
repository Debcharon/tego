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
