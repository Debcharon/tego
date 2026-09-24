// Package config reads and validates startup configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Debcharon/tego/internal/verification"
)

type Config struct {
	Token        string
	AdminID      int64
	Lang         string
	Verification *verification.Config
}

func Load() (Config, error) {
	cfg := Config{Token: strings.TrimSpace(os.Getenv("BOT_TOKEN")), Lang: os.Getenv("BOT_LANG")}
	if cfg.Token == "" {
		return Config{}, fmt.Errorf("BOT_TOKEN is required")
	}
	id, err := strconv.ParseInt(os.Getenv("ADMIN_ID"), 10, 64)
	if err != nil || id <= 0 {
		return Config{}, fmt.Errorf("ADMIN_ID must be a positive Telegram user ID")
	}
	cfg.AdminID = id
	if cfg.Lang == "" {
		cfg.Lang = "en"
	}
	switch cfg.Lang {
	case "en", "zh_cn", "zh_cn_moe":
	default:
		return Config{}, fmt.Errorf("unsupported BOT_LANG %q", cfg.Lang)
	}
	cfg.Verification, err = verification.New(os.Getenv("VERIFY_URL"), os.Getenv("VERIFY_SIGNING_KEY"))
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}
