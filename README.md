# tego

English | [简体中文](README_CN.md)

A single-admin Telegram message relay bot written in Go. The admin replies to forwarded messages to answer users anonymously.

## Setup

Install Go 1.25 or later and create a Telegram bot. Configure the process environment:

```dotenv
BOT_TOKEN=your_bot_token
ADMIN_ID=123456789
BOT_LANG=en
```

`BOT_TOKEN` and `ADMIN_ID` are required; use your positive numeric Telegram user ID for `ADMIN_ID`. `BOT_LANG` defaults to `en` and also supports `zh_cn` and `zh_cn_moe`. Configuration comes from environment variables, not `.env` files loaded by the binary.

For local PowerShell execution:

```powershell
$env:BOT_TOKEN = "your_bot_token"
$env:ADMIN_ID = "123456789"
$env:BOT_LANG = "en"
go run .
```

For Bash, export the variables and run `go run .`. Use `-data-dir /path/to/data` to select the SQLite directory. Build with `go build -trimpath -ldflags="-s -w" -o bot .`.

For Docker Compose, copy `.env.example` to `.env`, set `BOT_TOKEN` and `ADMIN_ID`, then run `docker compose up -d`. Compose mounts `data/` for persistence and pulls `microcharon/tego:latest`.

## Commands

| Command | Purpose |
| --- | --- |
| `/start` | Visitor introduction or verification; admin opens the inline-button panel |
| `/help` | Usage help for visitors or admin |
| `/status` | Running status; admin also sees version and verification mode |
| `/notification` | Toggle delivery confirmation for yourself |
| `/info` | Admin: reply to a forwarded message to see sender |
| `/ban` | Admin: reply to a forwarded message or provide user ID |
| `/unban` | Admin: reply to a forwarded message or provide user ID |
| `/banlist` | Admin: browse banned users |
| `/unverify` | Admin: reply to a forwarded message or provide user ID to revoke verification |

The visitor menu shows `/start`, `/help`, and `/notification`; the admin menu shows `/start`, `/help`, `/info`, `/ban`, and `/unban`. Other commands remain available by typing them. `/start` opens the admin panel with user lists, ban and verification controls, status, and notification settings. Panel buttons work only in the admin's private chat.

The bot stores preferences, bans, verification state, message mappings, and polling offset in `data/bot.db` (SQLite). Run only one bot instance per token and data directory.

Delivered update IDs prevent most duplicate actions after an update replay, but a crash after Telegram accepts a message can still cause a duplicate on retry.

At startup and daily, the bot removes expired verification challenges and message mappings older than 180 days; replies through removed mappings no longer work. Preferences, bans, and verification state are retained. Back up `data/bot.db` before maintenance, and stop the bot before running `VACUUM`. Telegram's `retry_after` is respected when rate limited.

## Optional visitor verification

Set both `VERIFY_URL` and `VERIFY_SIGNING_KEY` to enable visitor verification. The [tego-verify](https://github.com/Debcharon/tego-verify) Mini App handles Cloudflare Turnstile or hCaptcha. Use its production HTTPS URL and share a key generated with `openssl rand -hex 32` with the Vercel project. Keep the signing key and CAPTCHA secrets out of Git.

When enabled, visitors must pass the CAPTCHA before relaying messages; `/start` requests the verification button, and `/help` and `/status` remain available. After verification, users must resend their message. The admin is exempt, but bans still apply. Invalid verification settings stop startup; service outages do not bypass verification.

For Docker Compose, put both variables in `.env` or export them. Select `CAPTCHA_PROVIDER` and its keys in tego-verify; the bot needs no provider setting.

## Releases and containers

A `v1.YYYYMMDD.N` tag on `master` triggers tests, GitHub Release archives and checksums, and versioned Docker Hub and GHCR images.

Use `microcharon/tego:latest` or pin a published version tag in Compose. Check the intended commit before pushing a release tag.
