package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	dataDir := flag.String("data-dir", "data", "directory containing config.json and bot.db")
	flag.Parse()
	store, err := loadStore(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("set BOT_TOKEN")
	}
	lang, err := loadLanguage(store.Config.Lang)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	telegram := newTelegram(token)
	me, err := telegram.getMe(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if err := telegram.setCommands(ctx); err != nil {
		log.Printf("set commands failed: %v", err)
	}
	bot := &Bot{store: store, api: telegram, lang: lang, username: me.Username}
	log.Printf("bot started: id=%d username=@%s", me.ID, me.Username)
	if err := bot.run(ctx, telegram); err != nil {
		log.Fatal(err)
	}
}
