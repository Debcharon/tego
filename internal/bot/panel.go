package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Debcharon/tego/internal/telegram"
)

func button(text, data string) telegram.Button { return telegram.Button{Text: text, Data: data} }

func (b *Bot) panel(ctx context.Context, chatID int64, route string) error {
	text, buttons, err := b.panelPage(route)
	if err != nil {
		return err
	}
	return b.api.SendPanel(ctx, chatID, text, buttons)
}

func (b *Bot) panelPage(route string) (string, [][]telegram.Button, error) {
	back := []telegram.Button{button(b.text("panel_back"), "home")}
	switch {
	case route == "home":
		return fmt.Sprintf(b.text("panel_home"), b.version), [][]telegram.Button{
			{button(b.text("panel_users"), "users:0"), button(b.text("panel_bans"), "bans:0")},
			{button(b.text("panel_verified"), "verified:0"), button(b.text("panel_status"), "status")},
			{button(b.text("panel_settings"), "settings")},
		}, nil
	case route == "status":
		stats, err := b.store.Statistics()
		if err != nil {
			return "", nil, err
		}
		mode := b.text("status_verification_off")
		if b.verify != nil {
			mode = b.text("status_verification_on")
		}
		return fmt.Sprintf(b.text("panel_status_detail"), b.version, mode, stats.Users, stats.Banned, stats.Verified, stats.ReplyableMessages), [][]telegram.Button{back}, nil
	case route == "settings":
		state := b.text("notification_off")
		if b.store.Preference(b.adminID).Notification {
			state = b.text("notification_on")
		}
		return b.text("panel_settings") + "\n" + state, [][]telegram.Button{
			{button(b.text("panel_toggle_notification"), "toggle")}, back,
		}, nil
	case strings.HasPrefix(route, "users:") || strings.HasPrefix(route, "bans:") || strings.HasPrefix(route, "verified:"):
		parts := strings.Split(route, ":")
		page, err := strconv.Atoi(parts[1])
		if err != nil || page < 0 || page > 100000 {
			return b.text("panel_invalid"), [][]telegram.Button{back}, nil
		}
		filter := "all"
		title := b.text("panel_users")
		if parts[0] == "bans" {
			filter, title = "blocked", b.text("panel_bans")
		}
		if parts[0] == "verified" {
			filter, title = "verified", b.text("panel_verified")
		}
		users, more, err := b.store.Users(page, filter)
		if err != nil {
			return "", nil, err
		}
		text := fmt.Sprintf("%s · %d\n", title, page+1)
		if len(users) == 0 {
			text += b.text("panel_empty")
		}
		buttons := make([][]telegram.Button, 0, len(users)+2)
		for _, u := range users {
			name := strings.NewReplacer("\n", " ", "\r", " ").Replace(u.Name)
			if len([]rune(name)) > 24 {
				name = string([]rune(name)[:24]) + "…"
			}
			buttons = append(buttons, []telegram.Button{button(fmt.Sprintf("%s · %d", name, u.ID), fmt.Sprintf("user:%d", u.ID))})
		}
		nav := []telegram.Button{}
		if page > 0 {
			nav = append(nav, button("◀", fmt.Sprintf("%s:%d", parts[0], page-1)))
		}
		if more {
			nav = append(nav, button("▶", fmt.Sprintf("%s:%d", parts[0], page+1)))
		}
		if len(nav) > 0 {
			buttons = append(buttons, nav)
		}
		buttons = append(buttons, back)
		return text, buttons, nil
	case strings.HasPrefix(route, "user:"):
		id, err := strconv.ParseInt(strings.TrimPrefix(route, "user:"), 10, 64)
		if err != nil || id <= 0 {
			return b.text("panel_invalid"), [][]telegram.Button{back}, nil
		}
		u, found, err := b.store.User(id)
		if err != nil {
			return "", nil, err
		}
		if !found {
			return b.text("user_not_found"), [][]telegram.Button{back}, nil
		}
		blocked, verified := b.text("panel_no"), b.text("panel_no")
		if u.Blocked {
			blocked = b.text("panel_yes")
		}
		if u.Verified {
			verified = b.text("panel_yes")
		}
		text := fmt.Sprintf(b.text("panel_user_detail"), u.Name, u.ID, blocked, verified)
		buttons := [][]telegram.Button{}
		if id != b.adminID {
			action, label := "ban", b.text("panel_ban")
			if u.Blocked {
				action, label = "unban", b.text("panel_unban")
			}
			buttons = append(buttons, []telegram.Button{button(label, fmt.Sprintf("confirm:%s:%d", action, id))})
			if u.Verified {
				buttons = append(buttons, []telegram.Button{button(b.text("panel_unverify"), fmt.Sprintf("confirm:unverify:%d", id))})
			}
		}
		buttons = append(buttons, back)
		return text, buttons, nil
	case strings.HasPrefix(route, "confirm:"):
		parts := strings.Split(route, ":")
		if len(parts) != 3 {
			return b.text("panel_invalid"), [][]telegram.Button{back}, nil
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || id <= 0 || id == b.adminID || !validPanelAction(parts[1]) {
			return b.text("panel_invalid"), [][]telegram.Button{back}, nil
		}
		u, found, err := b.store.User(id)
		if err != nil {
			return "", nil, err
		}
		if !found {
			return b.text("user_not_found"), [][]telegram.Button{back}, nil
		}
		return fmt.Sprintf(b.text("panel_confirm"), b.panelActionLabel(parts[1]), u.Name, id), [][]telegram.Button{
			{button(b.text("panel_confirm_yes"), "do:"+parts[1]+":"+parts[2]), button(b.text("panel_cancel"), "user:"+parts[2])},
		}, nil
	}
	return b.text("panel_invalid"), [][]telegram.Button{back}, nil
}

func validPanelAction(action string) bool {
	return action == "ban" || action == "unban" || action == "unverify"
}

func (b *Bot) panelActionLabel(action string) string {
	switch action {
	case "ban":
		return b.text("panel_ban")
	case "unban":
		return b.text("panel_unban")
	default:
		return b.text("panel_unverify")
	}
}

func (b *Bot) callback(ctx context.Context, query *telegram.CallbackQuery, updateID int64) error {
	if query == nil {
		return nil
	}
	if query.From.ID != b.adminID || query.Message == nil || query.Message.Chat.ID != b.adminID || query.Message.Chat.Type != "private" {
		return b.api.AnswerCallback(ctx, query.ID, b.text("not_an_admin"), true)
	}
	if err := b.api.AnswerCallback(ctx, query.ID, "", false); err != nil {
		return err
	}
	route := query.Data
	if route == "toggle" {
		p := b.store.Preference(b.adminID)
		p.Notification = !p.Notification
		if err := b.store.SetPreferenceForUpdate(updateID, b.adminID, p); err != nil {
			return err
		}
		route = "settings"
	} else if strings.HasPrefix(route, "do:") {
		parts := strings.Split(route, ":")
		if len(parts) != 3 || !validPanelAction(parts[1]) {
			route = "home"
		} else {
			id, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil || id <= 0 || id == b.adminID {
				route = "home"
			} else {
				_, found, err := b.store.User(id)
				if err != nil {
					return err
				}
				if !found {
					route = "home"
				} else {
					if parts[1] == "unverify" {
						if _, err := b.store.RevokeVerification(updateID, id); err != nil {
							return err
						}
					} else {
						p := b.store.Preference(id)
						p.Blocked = parts[1] == "ban"
						if err := b.store.SetPreferenceForUpdate(updateID, id, p); err != nil {
							return err
						}
						key := "be_blocked_alert"
						if parts[1] == "unban" {
							key = "be_unbanned"
						}
						b.notify(ctx, id, b.text(key))
					}
					route = "user:" + parts[2]
				}
			}
		}
	}
	text, buttons, err := b.panelPage(route)
	if err != nil {
		return err
	}
	err = b.api.EditPanel(ctx, b.adminID, query.Message.MessageID, text, buttons)
	var apiErr *telegram.APIError
	if errors.As(err, &apiErr) && apiErr.Code == 400 && strings.Contains(apiErr.Description, "message is not modified") {
		return nil
	}
	return err
}
