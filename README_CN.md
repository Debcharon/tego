# tego

[English](README.md) | 简体中文

这是一个 Go 编写的单管理员 Telegram 消息中转机器人。管理员回复转发消息即可匿名答复用户。

## 配置与运行

安装 Go 1.25 或更新版本并创建 Telegram 机器人，通过环境变量配置：

```dotenv
BOT_TOKEN=your_bot_token
ADMIN_ID=123456789
BOT_LANG=en
```

`BOT_TOKEN` 和 `ADMIN_ID` 必填；`ADMIN_ID` 填自己的 Telegram 正整数用户 ID。`BOT_LANG` 默认 `en`，另支持 `zh_cn` 和 `zh_cn_moe`。程序从环境变量读取配置，不会自动加载 `.env` 文件。

本地 PowerShell 运行示例：

```powershell
$env:BOT_TOKEN = "your_bot_token"
$env:ADMIN_ID = "123456789"
$env:BOT_LANG = "en"
go run .
```

Bash 环境先导出这些变量，再运行 `go run .`。通过 `-data-dir /path/to/data` 指定 SQLite 目录。编译命令：`go build -trimpath -ldflags="-s -w" -o bot .`。

Docker Compose：将 `.env.example` 复制为 `.env`，填写 `BOT_TOKEN` 和 `ADMIN_ID`，再运行 `docker compose up -d`。Compose 挂载 `data/` 保存数据，默认拉取 `microcharon/tego:latest`。

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

访客菜单显示 `/start`、`/help` 和 `/notification`；管理员菜单显示 `/start`、`/help`、`/info`、`/ban` 和 `/unban`。其他命令仍可手动输入。管理员用 `/start` 打开面板，可查看用户列表、管理封禁与验证、查看状态和设置消息确认提示。面板按钮仅在管理员私聊中生效。

用户设置、封禁、验证状态、消息映射和长轮询位置保存在 SQLite 数据库 `data/bot.db`。同一 Token 和数据目录只运行一个机器人实例。

数据库会记录已投递的更新 ID，避免更新重放造成多数重复操作；但 Telegram 接收消息后若程序立即崩溃，重试仍可能重复发送。

程序启动时及之后每天会清理过期验证挑战和超过 180 天的消息映射；映射删除后无法再通过对应消息回复。用户设置、封禁和验证状态会保留。维护数据库前备份 `data/bot.db`，运行 `VACUUM` 前先停止机器人。遇到限流时，程序会遵循 Telegram 返回的 `retry_after`。

## 可选访客验证

同时设置 `VERIFY_URL` 和 `VERIFY_SIGNING_KEY` 即可启用访客验证。[tego-verify](https://github.com/Debcharon/tego-verify) Mini App 提供 Cloudflare Turnstile 或 hCaptcha 验证。`VERIFY_URL` 填正式 HTTPS 地址；用 `openssl rand -hex 32` 生成签名密钥，并在 Vercel 项目设置相同的密钥。不要将签名密钥或验证码私钥提交到 Git。

启用后，访客须通过验证才能转发消息；`/start` 可重新获取验证按钮，`/help` 和 `/status` 仍可使用。验证成功后需重新发送消息。管理员免验证，但封禁仍生效。验证配置无效时程序拒绝启动；验证服务不可用时不会放行。

使用 Docker Compose 时，将两个变量写入 `.env` 或提前导出。在 tego-verify 配置 `CAPTCHA_PROVIDER` 和对应密钥即可切换验证码服务；机器人无需设置服务类型。

## 版本发布与容器镜像

在 `master` 上推送 `v1.YYYYMMDD.N` 格式的标签，会触发测试、GitHub Release 程序包与校验和发布，以及 Docker Hub 和 GHCR 的版本镜像发布。

Compose 可使用 `microcharon/tego:latest`，也可固定到已发布的版本标签。推送发布标签前请确认提交。
