package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed lang/*.json
var languages embed.FS

func Load(name string) (map[string]string, error) {
	if name != "en" && name != "zh_cn" && name != "zh_cn_moe" {
		return nil, fmt.Errorf("unsupported language %q", name)
	}
	data, err := languages.ReadFile("lang/" + name + ".json")
	if err != nil {
		return nil, err
	}
	var lang map[string]string
	if err := json.Unmarshal(data, &lang); err != nil {
		return nil, err
	}
	for _, key := range []string{"start", "notification_on", "notification_off", "info_data", "message_received_notification", "reply_to_no_message", "reply_to_message_no_data", "reply_type_not_supported", "reply_message_sent", "please_setup_first", "blocked_alert", "reply_message_failed", "be_blocked_alert", "ban_user", "unban_user", "nonexistent_command", "not_an_admin", "reply_or_enter_id", "user_not_found", "be_unbanned", "verification_required", "verification_button", "verification_success", "verification_failed", "help_user", "help_admin", "status_user", "status_admin", "status_verification_on", "status_verification_off", "unverify_done", "unverify_not_verified", "panel_home", "panel_users", "panel_bans", "panel_verified", "panel_status", "panel_settings", "panel_back", "panel_empty", "panel_invalid", "panel_status_detail", "panel_toggle_notification", "panel_yes", "panel_no", "panel_user_detail", "panel_ban", "panel_unban", "panel_unverify", "panel_confirm", "panel_confirm_yes", "panel_cancel"} {
		if lang[key] == "" {
			return nil, fmt.Errorf("language %s missing %s", name, key)
		}
	}
	return lang, nil
}
