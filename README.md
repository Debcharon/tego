# tego

English | [简体中文](README_CN.md)

A single-admin private message relay bot written in Go. User messages are forwarded to the admin; the admin replies to a forwarded message to answer the user anonymously. It uses long polling and the Telegram Bot API directly, with a pure-Go SQLite driver for persistence.

## Setup

Install Go 1.25 or later and create a Telegram bot. Copy `data/config.example.json` to `data/config.json`, then set your admin ID:

```json
{"admin": 123456789, "lang": "en"}
```

Set the bot token through the `BOT_TOKEN` environment variable. The config file is local and ignored by Git. `lang` can be `en`, `zh_cn`, or `zh_cn_moe`. `admin` must be your numeric Telegram user ID; the bot will not start when it is unset.

Run `go run .` from the project root. The language files are embedded in the binary; use `-data-dir /path/to/data` to run it from another working directory. To build a portable executable, use `go build -trimpath -ldflags="-s -w" -o bot .`; set `GOOS` and `GOARCH` for other platforms.

Docker: after creating `data/config.json` and setting `BOT_TOKEN`, run `docker compose up -d`. Compose pulls `microcharon/tego:latest` from Docker Hub on each run. The `data` directory is mounted for persistent settings and mappings.

## Commands

| Command | Purpose |
| --- | --- |
| `/start` | Introduction |
| `/help` | Project information |
| `/ping` | Health response |
| `/notification` | Toggle delivery confirmation for yourself |
| `/info` | Admin: reply to a forwarded message to see sender |
| `/ban` | Admin: reply to a forwarded message to ban sender |
| `/unban` | Admin: reply to a forwarded message or provide user ID |

Messages, media, and captions supported by Telegram's `copyMessage` can be replied to. Admin replies do not expose the admin's account to the user. Preferences, message mappings, and polling offset are stored in `data/bot.db` (SQLite). The Go version starts with a new database and does not import Python JSON data. Run only one bot instance against a data directory and token.

SQLite records delivered update IDs to avoid repeating a successful relay or state change when an update is replayed. A network failure or process crash between Telegram accepting a message and SQLite recording it can still cause a duplicate on retry; Telegram does not provide an idempotency key for these sends. Optional confirmation messages are best effort and never trigger a second relay.
