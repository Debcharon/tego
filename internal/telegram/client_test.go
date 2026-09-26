package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRetryAfterIsParsed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":17}}`))
	}))
	defer server.Close()
	api := New("test")
	api.baseURL = server.URL + "/"
	err := api.Send(context.Background(), 1, "hello", 0)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != 429 || apiErr.RetryAfter != 17 {
		t.Fatalf("retry_after: %v", err)
	}
}

func TestGetUpdatesIncludesCallbacks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Allowed []string `json:"allowed_updates"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Allowed) != 2 || body.Allowed[0] != "message" || body.Allowed[1] != "callback_query" {
			t.Errorf("allowed updates: %v", body.Allowed)
		}
		w.Write([]byte(`{"ok":true,"result":[{"update_id":10,"callback_query":{"id":"a","from":{"id":1},"message":{"message_id":5,"chat":{"id":1,"type":"private"}},"data":"home"}}]}`))
	}))
	defer server.Close()
	api := New("test")
	api.baseURL = server.URL + "/"
	updates, err := api.GetUpdates(context.Background(), 10)
	if err != nil || len(updates) != 1 || updates[0].CallbackQuery == nil || updates[0].CallbackQuery.Data != "home" {
		t.Fatalf("callbacks: %+v %v", updates, err)
	}
}

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
		if r.URL.Path != "/setMyCommands" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		calls = append(calls, body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{\"ok\":true,\"result\":true}"))
	}))
	defer server.Close()
	api := New("test")
	api.baseURL = server.URL + "/"
	for _, language := range []string{"en", "zh_cn", "zh_cn_moe"} {
		calls = nil
		if err := api.SetCommands(context.Background(), 42, language); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 2 {
			t.Fatalf("%s: got %d calls", language, len(calls))
		}
		visitorScope := calls[0]["scope"].(map[string]any)
		adminScope := calls[1]["scope"].(map[string]any)
		if visitorScope["type"] != "all_private_chats" || adminScope["type"] != "chat" || adminScope["chat_id"] != float64(42) {
			t.Fatalf("%s: bad scopes: %+v", language, calls)
		}
		for index, expected := range [][]string{{"start", "help", "notification"}, {"start", "help", "info", "ban", "unban"}} {
			var actual []string
			for _, raw := range calls[index]["commands"].([]any) {
				actual = append(actual, raw.(map[string]any)["command"].(string))
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: menu %d: got %v, want %v", language, index, actual, expected)
			}
		}
	}
}
