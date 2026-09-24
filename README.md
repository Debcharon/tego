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

## Optional visitor verification

Verification is disabled unless both `VERIFY_URL` and `VERIFY_SIGNING_KEY` are set. The separate [tego-verify](https://github.com/Debcharon/tego-verify) project hosts the HTTPS Telegram Mini App on Vercel and checks Cloudflare Turnstile. Set `VERIFY_URL` to its production HTTPS URL and generate a shared signing key with `openssl rand -hex 32`. Set the same key in the Vercel project. Never commit the key or the Turnstile secret.

When enabled, non-admin users must open the verification button and pass Turnstile before messages or commands are relayed. The admin is exempt; bans still apply. Verified user IDs and outstanding challenges are stored in `data/bot.db`; the Vercel page does not access the bot's database. A successful check returns a one-time signed proof through Telegram. The bot then asks the user to resend the original message. If either required variable is missing or invalid, the bot refuses to start; an unavailable verification service does not bypass the gate.

For Docker Compose, export the two variables or put them in a local `.env` file before `docker compose up -d`. Compose uses the published Hub image, so publish an image containing this change before enabling verification in that deployment.

## Releases and containers

A version tag in the form `v1.YYYYMMDD.N` on `master` runs tests, publishes Linux (AMD64/ARM64), Windows (AMD64), and macOS (AMD64/ARM64) archives with `checksums.txt` in GitHub Releases, then publishes matching multi-platform container tags to Docker Hub and GitHub Container Registry. Release binaries include the example configuration file; they never include a bot token or database.

The existing `microcharon/tego:latest` image continues to be published from `master`. For a pinned version, use `microcharon/tego:v1.20260924.0` or `ghcr.io/debcharon/tego:v1.20260924.0` in Compose. GitHub Container Registry packages are private on first publish by default; set the package visibility to public in GitHub if anonymous pulls are desired. Use .0 for the first release of a day, then .1, .2, and so on. Creating and pushing a release tag publishes the artifacts, so verify the intended commit and version before tagging.
