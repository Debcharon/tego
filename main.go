package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Debcharon/tego/internal/bot"
	"github.com/Debcharon/tego/internal/config"
	"github.com/Debcharon/tego/internal/i18n"
	"github.com/Debcharon/tego/internal/store"
	"github.com/Debcharon/tego/internal/telegram"
)

var version = "v1.20260924.0-dev"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dataDir := flag.String("data-dir", "data", "directory containing bot.db")
	flag.Parse()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	language, err := i18n.Load(cfg.Lang)
	if err != nil {
		return err
	}
	db, err := store.Open(*dataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := telegram.New(cfg.Token)
	me, err := client.GetMe(ctx)
	if err != nil {
		return err
	}
	if err := client.SetCommands(ctx, cfg.AdminID, cfg.Lang); err != nil {
		log.Printf("set commands failed: %v", err)
	}
	relay := bot.New(db, client, language, me.Username, cfg.AdminID, cfg.Verification, version)
	log.Printf("bot started: id=%d username=@%s", me.ID, me.Username)
	if err := relay.Run(ctx, client); err != nil {
		return err
	}
	return nil
}
