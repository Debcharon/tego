package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name, token, admin, lang, url, key string
		wantError                          bool
	}{
		{name: "defaults", token: "test", admin: "123"},
		{name: "Chinese", token: "test", admin: "123", lang: "zh_cn"},
		{name: "moe", token: "test", admin: "123", lang: "zh_cn_moe"},
		{name: "missing token", admin: "123", wantError: true},
		{name: "missing admin", token: "test", wantError: true},
		{name: "zero admin", token: "test", admin: "0", wantError: true},
		{name: "negative admin", token: "test", admin: "-1", wantError: true},
		{name: "invalid admin", token: "test", admin: "abc", wantError: true},
		{name: "overflow admin", token: "test", admin: "9223372036854775808", wantError: true},
		{name: "invalid language", token: "test", admin: "123", lang: "fr", wantError: true},
		{name: "verification URL only", token: "test", admin: "123", url: "https://example.com", wantError: true},
		{name: "verification key only", token: "test", admin: "123", key: strings.Repeat("00", 32), wantError: true},
		{name: "verification enabled", token: "test", admin: "123", url: "https://example.com", key: strings.Repeat("00", 32)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range map[string]string{"BOT_TOKEN": tt.token, "ADMIN_ID": tt.admin, "BOT_LANG": tt.lang, "VERIFY_URL": tt.url, "VERIFY_SIGNING_KEY": tt.key} {
				t.Setenv(k, v)
			}
			cfg, err := Load()
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, want error %v", err, tt.wantError)
			}
			if tt.wantError {
				return
			}
			wantLang := tt.lang
			if wantLang == "" {
				wantLang = "en"
			}
			if cfg.AdminID != 123 || cfg.Lang != wantLang || cfg.Token != tt.token {
				t.Fatalf("unexpected parsed configuration")
			}
			if (cfg.Verification != nil) != (tt.url != "") {
				t.Fatal("incorrect verification state")
			}
		})
	}
}
