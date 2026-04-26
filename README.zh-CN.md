# imtty

[English](README.md) | 简体中文

`imtty` 是一个 Telegram-first 的本地桥接器，用来从手机或任意 Telegram 客户端运行和控制本机 `tmux` 里的 Codex 会话。

它刻意保持窄范围：

- 单用户
- 单机部署
- Telegram 作为主要远程入口
- `tmux` 作为唯一会话承载层
- Codex 运行在 `tmux` 内
- 保留 human-in-the-loop 审批流

这个项目不是多用户 agent 平台，也不是托管控制面或通用文件同步产品。

## 当前状态

仓库已经有可运行基线，不只是设计文档。

当前已实现：

- Telegram webhook bridge
- `tmux` 会话生命周期管理
- Codex `app-server` 集成
- Telegram 默认只发 final answer
- Telegram Mini App 控制面板
- 动态项目白名单
- 图片输入：
  - Telegram `photo`
  - 图片型 `document`
- 文件分析输入：
  - 文本和代码文件
  - PDF 文本提取
- turn 处理中 Telegram 原生 `typing`
- bridge 重启后会话 reattach
- 本地 attach 保护：
  - 本地可写 attach 会阻断 Telegram 继续写入
  - 本地只读旁观不会阻断 Telegram 继续写入

## 解决的问题

离开桌面后，你仍然可能想要：

- 再给 Codex 发一条 prompt
- 看最终回复
- 批准命令或权限请求
- 在多个项目会话之间切换
- 用轻量 UI 查看和管理会话

`imtty` 提供这些能力，但不会暴露原始 shell，也不会引入会改变 Codex 交互模型的自主编排层。

## 架构

```text
Telegram -> imtty bridge -> tmux session -> Codex app-server
                              |
                              +-> Telegram Mini App
```

关键设计选择：

- `tmux` 是唯一 session backend
- Telegram 默认输出来自 Codex 结构化事件，而不是终端屏幕抓取
- 审批保持显式
- 一个 Telegram chat 任一时刻只绑定一个 active session
- 可以同时存在多个 project session

## 功能

### 会话

- `/open <project>` 创建或绑定 `codex-{project}`
- `/open <project> <thread-id>` 严格恢复给定 Codex thread id，并将其绑定为该项目当前 thread
- `/close` 解除当前 active session 绑定
- `/kill` 终止底层会话并从已知会话列表删除
- `/close` 会在解除绑定前回显当前 thread id
- `/kill` 会在删除会话前回显当前 thread id
- `/clear` 为当前 active session 立即创建一个新的空 thread，但不重建 tmux session
- `/status` 查看当前 active session 详情和当前 Codex 窗口统计
- `/list` 只列会话
- `/projects` 只列允许打开的项目
- `/model [model-id]` 查看或设置当前 active session 的模型覆盖
- `/reasoning [effort]` 查看或设置当前 active session 的 reasoning 覆盖
- `/plan_mode [default|plan]` 查看或设置当前 active session 的 bridge 侧计划预设

### 项目白名单

- `/project_add <name> <abs-path>`
- `/project_remove <name>`
- 动态项目会持久化到本地 `.imtty-projects.json`
- 静态项目仍来自配置文件或环境变量

### Telegram 聊天体验

- 普通文本发到当前 active session
- 提交成功默认静默
- Codex 工作中 Telegram 会显示原生 `typing`
- 默认只回 final answer、审批提示和必要状态
- 终端噪音默认压制
- model / reasoning / plan mode 的切换会先挂到当前 active session，并在下一条真实消息时生效
- `/clear` 会立刻把当前 active session 切到新的空 thread，同时保留 tmux session 和当前 effective 控制状态
- 本地可写 attach 会阻断远程写入和远程 `/kill`
- 只读旁观命令：

```bash
tmux attach -r -t codex-<project>
```

### 图片输入

支持：

- Telegram `photo`
- 图片型 `document`

行为：

- 下载到本机临时目录
- 作为 `localImage` 提交给 Codex
- caption 会和图片一起作为同一轮输入

### 文件分析输入

支持：

- 文本文件
- 代码文件
- PDF 文档

行为：

- 文件下载到本机临时目录
- 文本和代码文件直接读取内容，再作为文本输入发给 Codex
- PDF 先在 bridge 本地提取文本，再作为文本输入发给 Codex
- 不支持的二进制文件会被拒绝

当前限制：

- 不支持任意二进制附件
- 第一版不做扫描版 PDF OCR
- 不做长期媒体存储

### Mini App

Mini App 是 Telegram 内的轻量控制面板，不是第二个终端。

当前支持：

- 当前 active session
- session 列表
- project 列表
- open / close / kill
- project add / remove
- bridge 主机目录浏览器，用于选择项目根目录

当前不支持：

- 实时终端输出
- 自由文本 prompt 输入
- 替代审批流

## 运行要求

本机已验证基线：

- `tmux 3.6a`
- `codex-cli 0.125.0`
- `go 1.26.1`

还需要：

- Telegram bot token
- Telegram 可访问的 HTTPS webhook 地址
- `cloudflared` 或同类公网入口

## 快速开始

### 1. 准备配置

```bash
cp config.toml.example config.toml
```

至少要改这些字段：

- `telegram_bot_token`
- `telegram_webhook_secret`
- `telegram_owner_id`
- `mini_app_base_url`
- `[projects]`

### 2. 启动 bridge

```bash
GOCACHE=/tmp/imtty-go-build go run ./cmd/imtty-bridge
```

健康检查：

```bash
curl -s http://127.0.0.1:8080/healthz
```

### 3. 用 Cloudflare Tunnel 暴露公网入口

长期运行建议使用 fixed hostname 的 named tunnel，例如 `imtty.example.com`。

#### 3.1 登录 Cloudflare

```bash
cloudflared tunnel login
```

#### 3.2 创建 named tunnel

```bash
cloudflared tunnel create imtty
cloudflared tunnel list
```

记下 tunnel UUID 和 credentials 文件路径。

#### 3.3 绑定固定域名

```bash
cloudflared tunnel route dns imtty imtty.example.com
```

#### 3.4 写入 `~/.cloudflared/config.yml`

```yaml
tunnel: imtty
credentials-file: /Users/<you>/.cloudflared/<TUNNEL-UUID>.json

ingress:
  - hostname: imtty.example.com
    service: http://127.0.0.1:8080
  - service: http_status:404
```

#### 3.5 让 `imtty` 指向固定公网地址

在 `config.toml` 里写：

```toml
mini_app_base_url = "https://imtty.example.com"
```

#### 3.6 启动 tunnel

前台运行：

```bash
cloudflared tunnel run imtty
```

macOS 登录后自动启动：

```bash
cloudflared service install
```

macOS 开机自动启动：

```bash
sudo cloudflared service install
```

#### 3.7 设置 Telegram webhook

```bash
curl -X POST "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/setWebhook" \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://imtty.example.com/telegram/webhook",
    "secret_token": "'"${IMTTY_TELEGRAM_WEBHOOK_SECRET}"'"
  }'
```

#### 3.8 验证公网链路

```bash
curl -s "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/getWebhookInfo"
curl -s http://127.0.0.1:8080/healthz
```

期望结果：

- webhook URL 是 `https://imtty.example.com/telegram/webhook`
- 本地 `healthz` 返回 `ok`
- bot 菜单按钮会打开 `https://imtty.example.com/mini-app`

#### 3.9 仅开发阶段的 quick tunnel

只在短期本地联调时使用：

```bash
cloudflared tunnel --url http://127.0.0.1:8080
```

如果 `trycloudflare.com` 域名变化，你必须重新更新：

- Telegram webhook
- Mini App 基础地址

### 4. 开始和 bot 对话

典型流程：

```text
/projects
/open my-project
hello
```

## 配置

默认从当前工作目录读取 `config.toml`。

可覆盖方式：

- `-config /abs/path/to/config.toml`
- `-listen :9090`
- `IMTTY_*` 环境变量

注意：

- `config.toml` 已被 git 忽略
- 只能提交 `config.toml.example`

## 部署说明

- 长期运行请使用 named Cloudflare Tunnel + 固定域名
- 建议使用子域名，例如 `imtty.example.com`，不要直接占用根域
- `mini_app_base_url` 应与 tunnel 的公开地址保持一致
- bridge 和 tunnel 是两个独立进程，重启一个不会自动重启另一个
- 如果公网域名变化，需要同步刷新：
  - Telegram webhook
  - Mini App menu button 目标地址

## 对外接口

### Bot 命令

- `/list`
- `/projects`
- `/project_add <name> <abs-path>`
- `/project_remove <name>`
- `/open <project>`
- `/open <project> <thread-id>`
- `/close`
- `/kill`
- `/clear`
- `/status`
- `/model [model-id]`
- `/reasoning [effort]`
- `/plan_mode [default|plan]`

### HTTP 接口

- `POST /telegram/webhook`
- `GET /healthz`
- `GET /mini-app`
- `GET/POST /mini-app/api/*`

## 安全模型

这个项目面向“用户本人控制的单机”场景。

当前保护措施：

- Telegram webhook secret 校验
- Mini App `initData` + owner 校验
- 项目白名单
- 本地可写 attach 冲突保护
- 临时媒体文件不进入项目目录

## 更多文档

- [Product Requirements](docs/prd.md)
- [MVP Architecture Runbook](docs/mvp-architecture-runbook.md)
- [Mini App design spec](docs/superpowers/specs/2026-04-25-imtty-mini-app-design.md)
- [App-server rewrite design](docs/superpowers/specs/2026-04-25-imtty-app-server-rewrite-design.md)

Mini App 前端在 `web/mini-app/`，构建产物在 `web/mini-app/dist/`。Mini App 路由必须使用 `/mini-app/#/...` 这种 hash 模式，Go bridge 直接把 `dist` 当静态目录提供；`npm run build` 后刷新 Mini App 即可读取新文件，不需要因为前端静态文件变化而重启 bridge。

## License

MIT. See [LICENSE](LICENSE).
