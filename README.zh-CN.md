# imtty

[English](README.md) | 简体中文

`imtty` 是一个个人使用的 Telegram 桥接器，用来从任意 Telegram 客户端控制本机 Codex 会话。

它让 Codex 继续运行在你自己的机器上、运行在 `tmux` 内，同时把 prompt、审批、媒体输入和最终回复通过 Telegram 传递。它面向单用户远程使用场景，不暴露原始 shell，也不把项目扩展成自主 agent 平台。

## 功能

- 面向单 owner 的 Telegram webhook bridge
- 基于 `tmux` 的 Codex 会话，会话名固定为 `codex-{project}`
- Codex `app-server` 集成，使用结构化事件输出
- Telegram 默认只返回 final answer，压制终端噪音
- 保留显式人工审批，通过完整 Telegram 文本提示和手动回复处理
- 项目白名单，支持动态增删
- bridge 重启后接管已有会话
- 本地桌面可写 attach 冲突保护
- Telegram Mini App companion UI，用于会话和项目控制
- 支持 Telegram `photo` 和图片型 `document`
- 支持文本、代码和 PDF 文件分析，文件只临时落地
- 可选 Telegram `voice` 输入，通过本地 `ffmpeg` 和 `whisper.cpp` 转写

## 非目标

`imtty` 不做：

- 托管服务
- 多用户系统
- agent 编排平台
- 通用远程 shell
- Web 后台管理面板
- 文件同步或长期媒体存储

## 架构

```text
Telegram -> imtty bridge -> tmux session -> Codex app-server
                              |
                              +-> Telegram Mini App
```

核心规则：

- Telegram 是主要 IM 入口。
- `tmux` 是唯一 session backend。
- Codex 必须运行在 `tmux` session 内。
- Telegram 输出来自 Codex 结构化事件，不读取终端屏幕。
- 一个 Telegram chat 任一时刻只绑定一个 active session。
- 可以同时存在多个 project session。
- bridge 不会自动确认 Codex 审批。

## 运行要求

已验证本机基线：

- macOS
- Go `1.26.1`
- `tmux 3.6a`
- `codex-cli 0.125.0`
- Telegram bot token
- Telegram 可访问的 HTTPS webhook 地址，例如 Cloudflare Tunnel

可选语音输入：

- `ffmpeg`
- `whisper.cpp` 的 `whisper-cli`
- 本地 GGML whisper 模型

## 快速开始

### 1. 创建配置

```bash
cp config.toml.example config.toml
```

至少需要配置：

```toml
telegram_bot_token = "BOT_TOKEN"
telegram_webhook_secret = "SECRET"
telegram_owner_id = 123456789
mini_app_base_url = "https://imtty.example.com"

[projects]
demo = "/absolute/path/to/your/project"
```

`config.toml` 已被 git 忽略。只提交 `config.toml.example`，不要提交本机配置。

### 2. 启动 bridge

```bash
go run ./cmd/imtty-bridge
```

如果希望使用临时构建缓存：

```bash
GOCACHE=/tmp/imtty-go-build go run ./cmd/imtty-bridge
```

健康检查：

```bash
curl -s http://127.0.0.1:8080/healthz
```

### 3. 暴露 webhook

长期运行建议使用 named Cloudflare Tunnel：

```bash
cloudflared tunnel login
cloudflared tunnel create imtty
cloudflared tunnel route dns imtty imtty.example.com
cloudflared tunnel run imtty
```

示例 `~/.cloudflared/config.yml`：

```yaml
tunnel: imtty
credentials-file: /Users/<you>/.cloudflared/<tunnel-id>.json

ingress:
  - hostname: imtty.example.com
    service: http://127.0.0.1:8080
  - service: http_status:404
```

设置 Telegram webhook：

```bash
curl -X POST "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/setWebhook" \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://imtty.example.com/telegram/webhook",
    "secret_token": "'"${IMTTY_TELEGRAM_WEBHOOK_SECRET}"'"
  }'
```

验证：

```bash
curl -s "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/getWebhookInfo"
curl -s http://127.0.0.1:8080/healthz
```

### 4. 和 bot 对话

```text
/projects
/open demo
hello
```

## 语音输入

语音输入默认关闭。配置本地转写器后再启用：

```toml
[voice]
enabled = true
ffmpeg_bin = "ffmpeg"
whisper_bin = "/opt/whisper.cpp/build/bin/whisper-cli"
model_path = "/opt/whisper.cpp/models/ggml-large-v3-turbo.bin"
language = "zh"
```

行为：

- Telegram `voice` 下载到本机临时目录。
- `ffmpeg` 转成 16 kHz mono WAV。
- `whisper.cpp` 在本机完成转写。
- 转写文本按普通文本提交到当前 active Codex session。
- bridge 不长期保存语音文件。

语音输入不包含语音命令、语音审批、说话人识别、长音频任务或多段合并。

## Bot 命令

```text
/list
/projects
/project_add <name> <abs-path>
/project_remove <name>
/open <project> [thread-id]
/close
/kill
/clear
/status
/model [model-id]
/reasoning [effort]
/plan_mode [default|plan]
```

## Mini App

Mini App 是 Telegram 内的轻量 companion UI，支持：

- 当前 active session
- session 列表
- project 列表
- open / close / kill 控制
- project add / remove
- bridge 主机目录浏览器，用于选择项目根目录

它不替代 Telegram 聊天窗口、实时终端输出、自由文本输入或 Codex 审批流。

前端代码在 `web/mini-app/`。构建产物提交在 `web/mini-app/dist/`，Go bridge 会直接提供这些静态文件。

## 开发

运行 Go 测试：

```bash
GOPATH=/tmp/imtty-go GOMODCACHE=/tmp/imtty-go/pkg/mod GOCACHE=/tmp/imtty-go-build go test ./...
```

构建 Mini App：

```bash
cd web/mini-app
npm install
npm run build
```

## 安全说明

- 只能打开项目白名单内的路径。
- Telegram webhook 请求必须携带配置的 secret token。
- Mini App 请求必须通过 Telegram `initData` 校验和 owner 校验。
- 图片、文件和语音只作为本机临时文件保存。
- 当同一 `tmux` session 被本地桌面可写 attach 时，bridge 会拒绝 Telegram 写入和 `/kill`。
- bridge 不会自动确认 Codex 权限审批。

## 更多文档

- [Product Requirements](docs/prd.md)
- [MVP Architecture Runbook](docs/mvp-architecture-runbook.md)
- [Guardrails](AGENTS.md)

## License

[MIT](LICENSE)
