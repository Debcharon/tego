# tego

[English](README.md) | 简体中文

本项目使用 Go 实现单管理员私聊中转：用户发给机器人的消息会转发给管理员，管理员回复转发消息即可匿名答复用户。程序通过长轮询直接使用 Telegram Bot API，并使用纯 Go 的 SQLite 驱动保存数据。

## 配置与运行

安装 Go 1.25 或更新版本并创建 Telegram 机器人。将 `data/config.example.json` 复制为 `data/config.json`，再填写管理员 ID：

```json
{"admin": 123456789, "lang": "zh_cn"}
```

只能通过环境变量 `BOT_TOKEN` 设置 Token；本地配置文件已被 Git 忽略。语言可选 `en`、`zh_cn`、`zh_cn_moe`。必须将 `admin` 设为自己的 Telegram 数字用户 ID；未设置时机器人不会启动。

在项目根目录运行 `go run .`。语言包已嵌入程序；在其他目录运行时可通过 `-data-dir /path/to/data` 指定数据目录。编译可执行文件：`go build -trimpath -ldflags="-s -w" -o bot .`；跨平台编译时设置 `GOOS` 和 `GOARCH`。

Docker：创建 `data/config.json` 并设置 `BOT_TOKEN` 后，执行 `docker compose up -d`。Compose 每次运行时都会从 Docker Hub 拉取 `microcharon/tego:latest`。`data` 目录会挂载保存设置和消息映射。

## 指令

| 指令 | 用途 |
| --- | --- |
| `/start` | 开始使用 |
| `/help` | 项目信息 |
| `/ping` | 运行状态 |
| `/notification` | 切换自己的消息确认提示 |
| `/info` | 管理员回复转发消息查询发送者 |
| `/ban` | 管理员回复转发消息封禁发送者 |
| `/unban` | 管理员回复转发消息或输入用户 ID 解封 |

管理员可回复 Telegram `copyMessage` 支持的消息、媒体和说明文字，答复不会暴露管理员账号。用户设置、消息映射和长轮询位置保存在 SQLite 数据库 `data/bot.db`。Go 版从空数据库开始，不导入旧版 Python 的 JSON 数据。同一 Token 和数据目录只应运行一个实例。

SQLite 会记录已投递的更新 ID，避免更新重放时再次执行已成功的转发或状态修改。若 Telegram 已接收消息、但程序在写入 SQLite 前断网或退出，重试仍可能造成重复；Telegram 发送接口没有可用的幂等键。可选确认提示发送失败不会触发再次转发。

## 可选访客验证

仅在同时设置 `VERIFY_URL` 和 `VERIFY_SIGNING_KEY` 后启用验证。独立的 [tego-verify](https://github.com/Debcharon/tego-verify) 项目在 Vercel 上提供 HTTPS Telegram Mini App，并通过 Cloudflare Turnstile 验证。将 `VERIFY_URL` 设为页面的正式 HTTPS 地址，用 `openssl rand -hex 32` 生成共享签名密钥，并在 Vercel 项目设置同一密钥。不要将密钥或 Turnstile 私钥提交到仓库。

启用后，非管理员用户须点击验证按钮并通过验证，才能继续向管理员发送消息。管理员免验证，封禁规则仍然生效。已验证用户 ID 和待完成的挑战保存在本地 `data/bot.db`；Vercel 页面不访问机器人的数据库。验证成功后，页面通过 Telegram 发送一次性签名凭证，机器人会提示用户重新发送原消息。两个必填变量缺失或无效时程序拒绝启动，验证服务不可用时也不会放行。

使用 Docker Compose 时，在运行 `docker compose up -d` 前导出这两个环境变量，或将其写入本地 `.env` 文件。Compose 使用已发布的 Docker Hub 镜像，因此启用前需先发布包含本次修改的镜像。

## 版本发布与容器镜像

在 `master` 上推送 `vMAJOR.MINOR.PATCH` 格式的版本标签后，Actions 会运行测试，将 Linux（AMD64/ARM64）、Windows（AMD64）和 macOS（AMD64/ARM64）的程序包及 `checksums.txt` 发布到 GitHub Releases，并将相同版本号的多架构镜像发布到 Docker Hub 和 GitHub Container Registry。程序包包含配置示例，不包含 Bot Token 或数据库。

现有 `microcharon/tego:latest` 仍由 `master` 更新。需要固定版本时，可在 Compose 中使用 `microcharon/tego:vX.Y.Z` 或 `ghcr.io/debcharon/tego:vX.Y.Z`。GHCR 首次发布的 Package 默认是私有的；如需匿名拉取，请在 GitHub 将其可见性改为 Public。推送版本标签会触发正式发布，因此打标签前应确认提交和版本号。
