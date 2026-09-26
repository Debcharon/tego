# tego

English | [简体中文](README_CN.md)

A single-admin private message relay bot written in Go. User messages are forwarded to the admin; the admin replies to a forwarded message to answer the user anonymously. It uses long polling and the Telegram Bot API directly, with a pure-Go SQLite driver for persistence.

## Setup

Install Go 1.25 or later and create a Telegram bot. Configure the process environment:

```dotenv
BOT_TOKEN=your_bot_token
ADMIN_ID=123456789
BOT_LANG=en
```

`BOT_TOKEN` and `ADMIN_ID` are required. `ADMIN_ID` must be your positive numeric Telegram user ID. `BOT_LANG` defaults to `en` and supports `en`, `zh_cn`, and `zh_cn_moe`. Invalid configuration stops startup before opening the database. Configuration is read only from environment variables; `data/config.json` is no longer loaded.

For local PowerShell execution:

```powershell
$env:BOT_TOKEN = "your_bot_token"
$env:ADMIN_ID = "123456789"
$env:BOT_LANG = "en"
go run .
```

For Bash, use `export BOT_TOKEN=... ADMIN_ID=123456789 BOT_LANG=en`, then `go run .`. The binary does not automatically load `.env` files. Language resources are embedded; use `-data-dir /path/to/data` to select the SQLite directory, which is created if missing. Build with `go build -trimpath -ldflags="-s -w" -o bot .`; set `GOOS` and `GOARCH` for cross-compilation.

For Docker Compose, copy `.env.example` to `.env`, fill in `BOT_TOKEN` and `ADMIN_ID`, then run `docker compose up -d`. Compose passes these variables into the container and mounts `data/` for persistence. `.env` is ignored by Git and excluded from Docker build context. Compose pulls `microcharon/tego:latest`; these changes require a published image containing them. To try this branch locally, build with `docker build -t tego:local .` and use `tego:local` with `pull_policy: never` in a local Compose override.

## Project structure

- `main.go`: process startup and version injection.
- `internal/config`: environment configuration and validation.
- `internal/bot`: commands, relay, polling, and verification interaction.
- `internal/telegram`: Telegram API client and message types.
- `internal/store`: SQLite persistence, delivery records, and verification state.
- `internal/verification`: signed verification tickets and proof validation.
- `internal/i18n`: embedded language resources.

Tests live alongside their packages. Run `go test ./...` and `go vet ./...` from the repository root.

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

The bot registers separate Telegram command menus for visitors and the admin at startup. The admin panel provides paginated user, banned-user, and verified-user lists; a user detail page with confirmation before changing bans or verification; status; and the admin's notification setting. Panel buttons work only in the configured admin's private chat.

Messages, media, and captions supported by Telegram's `copyMessage` can be replied to. Admin replies do not expose the admin's account to the user. Preferences, message mappings, and polling offset are stored in `data/bot.db` (SQLite). The Go version starts with a new database and does not import Python JSON data. Run only one bot instance against a data directory and token.

SQLite records delivered update IDs to avoid repeating a successful relay or state change when an update is replayed. A network failure or process crash between Telegram accepting a message and SQLite recording it can still cause a duplicate on retry; Telegram does not provide an idempotency key for these sends. Optional confirmation messages are best effort and never trigger a second relay.

At startup and once per day, the bot removes expired verification challenges and message mappings older than 180 days. Old mappings created before this version have no known timestamp and are retained; user preferences, bans, and verified-user records are never removed by this cleanup. Replies to a relayed message stop working after its mapping is removed. Back up `data/bot.db` before upgrading or doing manual database maintenance. SQLite may keep freed pages in the file until an offline `VACUUM`; stop the bot before running one. Telegram flood-control responses with `retry_after` are respected before polling or retrying an update.

## Optional visitor verification

Verification is disabled unless both `VERIFY_URL` and `VERIFY_SIGNING_KEY` are set. The separate [tego-verify](https://github.com/Debcharon/tego-verify) project hosts the HTTPS Telegram Mini App on Vercel and verifies either Cloudflare Turnstile or hCaptcha, selected in the tego-verify environment. Set `VERIFY_URL` to its production HTTPS URL and generate a shared signing key with `openssl rand -hex 32`. Set the same key in the Vercel project. Never commit the signing key or the selected CAPTCHA provider secret.

When enabled, non-admin users must open the verification button and pass the selected CAPTCHA before messages are relayed. They can still use /help and /status; /start requests a verification button. The admin is exempt; bans still apply. Verified user IDs and outstanding challenges are stored in `data/bot.db`; the Vercel page does not access the bot's database. A successful check returns a one-time signed proof through Telegram. The bot then asks the user to resend the original message. If either required variable is missing or invalid, the bot refuses to start; an unavailable verification service does not bypass the gate.

For Docker Compose, export the two variables or put them in a local `.env` file before `docker compose up -d`. To switch providers, set `CAPTCHA_PROVIDER` and the matching site and secret keys in tego-verify; the bot needs no provider setting or new image. Compose uses the published Docker Hub image.

## Releases and containers

A version tag in the form `v1.YYYYMMDD.N` on `master` runs tests, publishes Linux (AMD64/ARM64), Windows (AMD64), and macOS (AMD64/ARM64) archives with `checksums.txt` in GitHub Releases, then publishes matching multi-platform container tags to Docker Hub and GitHub Container Registry. Release binaries include the example configuration file; they never include a bot token or database.

The existing `microcharon/tego:latest` image continues to be published from `master`. For a pinned version, use `microcharon/tego:v1.20260924.0` or `ghcr.io/debcharon/tego:v1.20260924.0` in Compose. GitHub Container Registry packages are private on first publish by default; set the package visibility to public in GitHub if anonymous pulls are desired. Use .0 for the first release of a day, then .1, .2, and so on. Creating and pushing a release tag publishes the artifacts, so verify the intended commit and version before tagging.
