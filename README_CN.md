# tego

[English](README.md) | 简体中文

本项目使用 Go 实现单管理员私聊中转：用户发给机器人的消息会转发给管理员，管理员回复转发消息即可匿名答复用户。程序通过长轮询直接使用 Telegram Bot API，并使用纯 Go 的 SQLite 驱动保存数据。

## 配置与运行

安装 Go 1.25 或更新版本并创建 Telegram 机器人，通过环境变量配置：

```dotenv
BOT_TOKEN=your_bot_token
ADMIN_ID=123456789
BOT_LANG=en
```

`BOT_TOKEN` 和 `ADMIN_ID` 必填，管理员 ID 必须是自己的 Telegram 正整数用户 ID。`BOT_LANG` 默认 `en`，支持 `en`、`zh_cn` 和 `zh_cn_moe`。配置无效时，程序会在打开数据库前拒绝启动。配置仅从环境变量读取，不再加载 `data/config.json`。

本地 PowerShell 运行示例：

```powershell
$env:BOT_TOKEN = "your_bot_token"
$env:ADMIN_ID = "123456789"
$env:BOT_LANG = "en"
go run .
```

Bash 可以先运行 `export BOT_TOKEN=... ADMIN_ID=123456789 BOT_LANG=en`，再执行 `go run .`。程序不会自动加载 `.env`。语言资源已嵌入程序；通过 `-data-dir /path/to/data` 指定 SQLite 目录，目录不存在时自动创建。编译命令：`go build -trimpath -ldflags="-s -w" -o bot .`；跨平台编译时设置 `GOOS` 和 `GOARCH`。

Docker Compose：将 `.env.example` 复制为 `.env`，填写 `BOT_TOKEN` 和 `ADMIN_ID`，然后执行 `docker compose up -d`。Compose 会将变量传入容器，并挂载 `data/` 保存数据。`.env` 已被 Git 忽略，也不会进入 Docker 构建上下文。Compose 默认拉取 `microcharon/tego:latest`，因此需先发布包含本次修改的镜像。若要本地试用此分支，先执行 `docker build -t tego:local .`，再通过本地 Compose 覆盖文件将镜像设为 `tego:local`、`pull_policy` 设为 `never`。

## 项目结构

- `main.go`：程序启动与版本号注入。
- `internal/config`：环境变量读取和校验。
- `internal/bot`：命令、消息中转、长轮询和验证交互。
- `internal/telegram`：Telegram API 客户端和消息类型。
- `internal/store`：SQLite 持久化、投递记录和验证状态。
- `internal/verification`：验证凭证签名与校验。
- `internal/i18n`：嵌入式语言资源。

测试随所属包放置。在项目根目录运行 `go test ./...` 和 `go vet ./...`。

## 指令

| 指令 | 用途 |
| --- | --- |
| `/start` | 访客开始使用或请求验证；管理员打开按钮面板 |
| `/help` | 查看访客或管理员的使用帮助 |
| `/status` | 查看运行状态；管理员还能看到版本和验证开关 |
| `/notification` | 切换自己的消息确认提示 |
| `/info` | 管理员回复转发消息查询发送者 |
| `/ban` | 管理员回复转发消息或输入用户 ID 封禁 |
| `/unban` | 管理员回复转发消息或输入用户 ID 解封 |
| `/banlist` | 管理员分页查看封禁名单 |
| `/unverify` | 管理员回复转发消息或输入用户 ID 撤销验证 |

机器人启动时会分别设置访客和管理员的 Telegram 命令菜单。管理员面板提供用户、封禁和已验证用户分页列表；用户详情可在二次确认后封禁、解封或撤销验证；也可查看运行概况和切换管理员消息确认提示。面板按钮仅在指定管理员的私聊中生效。

管理员可回复 Telegram `copyMessage` 支持的消息、媒体和说明文字，答复不会暴露管理员账号。用户设置、消息映射和长轮询位置保存在 SQLite 数据库 `data/bot.db`。Go 版从空数据库开始，不导入旧版 Python 的 JSON 数据。同一 Token 和数据目录只应运行一个实例。

SQLite 会记录已投递的更新 ID，避免更新重放时再次执行已成功的转发或状态修改。若 Telegram 已接收消息、但程序在写入 SQLite 前断网或退出，重试仍可能造成重复；Telegram 发送接口没有可用的幂等键。可选确认提示发送失败不会触发再次转发。

程序启动时及之后每 24 小时会清理过期的验证挑战和超过 180 天的消息映射。旧版本的消息映射没有创建时间，无法判断年龄，因此保留；用户设置、封禁和已验证状态不会自动清理。映射被清理后，管理员无法再通过对应的旧转发消息回复用户。升级或手动维护数据库前请备份 `data/bot.db`。SQLite 删除记录后文件不一定立即缩小；如需运行 `VACUUM`，先停止机器人。遇到 Telegram 限流返回的 `retry_after` 时，轮询和更新重试会按该时间等待。

## 可选访客验证

仅在同时设置 `VERIFY_URL` 和 `VERIFY_SIGNING_KEY` 后启用验证。独立的 [tego-verify](https://github.com/Debcharon/tego-verify) 项目在 Vercel 上提供 HTTPS Telegram Mini App，并根据 tego-verify 的环境变量选用 Cloudflare Turnstile 或 hCaptcha 验证。将 `VERIFY_URL` 设为页面的正式 HTTPS 地址，用 `openssl rand -hex 32` 生成共享签名密钥，并在 Vercel 项目设置同一密钥。不要将共享签名密钥或所选验证码服务的私钥提交到仓库。

启用后，非管理员用户须点击验证按钮并通过验证，才能继续向管理员发送消息。未验证时仍可使用 /help、/status，发送 /start 可重新获取验证按钮。管理员免验证，封禁规则仍然生效。已验证用户 ID 和待完成的挑战保存在本地 `data/bot.db`；Vercel 页面不访问机器人的数据库。验证成功后，页面通过 Telegram 发送一次性签名凭证，机器人会提示用户重新发送原消息。两个必填变量缺失或无效时程序拒绝启动，验证服务不可用时也不会放行。

使用 Docker Compose 时，在运行 `docker compose up -d` 前导出这两个环境变量，或将其写入本地 `.env` 文件。切换验证码服务时，在 tego-verify 配置 `CAPTCHA_PROVIDER` 和对应的站点密钥、服务端私钥；机器人侧无需配置验证码服务类型。Compose 使用已发布的 Docker Hub 镜像；仅切换验证码服务不需要重新发布机器人镜像。

## 版本发布与容器镜像

在 `master` 上推送 `v1.YYYYMMDD.N` 格式的版本标签后，Actions 会运行测试，将 Linux（AMD64/ARM64）、Windows（AMD64）和 macOS（AMD64/ARM64）的程序包及 `checksums.txt` 发布到 GitHub Releases，并将相同版本号的多架构镜像发布到 Docker Hub 和 GitHub Container Registry。程序包包含配置示例，不包含 Bot Token 或数据库。

现有 `microcharon/tego:latest` 仍由 `master` 更新。需要固定版本时，可在 Compose 中使用 `microcharon/tego:v1.20260924.0` 或 `ghcr.io/debcharon/tego:v1.20260924.0`。GHCR 首次发布的 Package 默认是私有的；如需匿名拉取，请在 GitHub 将其可见性改为 Public。每天首次发布使用 .0，同一天再次发布时依次使用 .1、.2 等尾号。推送版本标签会触发正式发布，因此打标签前应确认提交和版本号。
