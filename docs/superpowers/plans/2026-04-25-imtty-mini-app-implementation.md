# imtty Mini App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `imtty` 增加一个 Telegram Mini App 轻量控制面板，提供 session 与 project 的结构化控制入口。

**Architecture:** Go bridge 新增 `internal/miniapp/` 负责 Telegram `initData` 校验、owner 鉴权、Mini App API 与静态资源服务；前端放在 `web/mini-app/`，使用 React + Vite + Tailwind CSS + shadcn/ui + Radix UI 构建单页控制面板。Mini App 只做结构化控制动作，仍复用现有 `session.Registry`、`tmux.Manager` 与 project store 作为真相源。

**Tech Stack:** Go 1.26、React、Vite、Tailwind CSS、shadcn/ui、Radix UI、Telegram Web Apps

---

### Task 1: 扩展配置与 Telegram bot 能力

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/telegram/bot.go`
- Modify: `internal/telegram/bot_test.go`

- [ ] 增加 `IMTTY_TELEGRAM_OWNER_ID` 与 `IMTTY_MINI_APP_BASE_URL` 配置解析和测试。
- [ ] 给 `BotClient` 增加设置 `Menu Button` 为 `web_app` 的能力及测试。

### Task 2: 先写 Mini App 鉴权红灯测试

**Files:**
- Create: `internal/miniapp/auth_test.go`
- Create: `internal/miniapp/http_test.go`

- [ ] 写 `initData` 校验通过的测试。
- [ ] 写 `hash` 错误时拒绝访问的测试。
- [ ] 写 owner 不匹配时返回 `403` 的测试。
- [ ] 写 `bootstrap` 返回 active session、sessions、projects 的测试。

### Task 3: 实现 Mini App 鉴权与 API

**Files:**
- Create: `internal/miniapp/auth.go`
- Create: `internal/miniapp/http.go`
- Create: `internal/miniapp/types.go`
- Modify: `internal/session/registry.go`
- Modify: `internal/session/registry_test.go`

- [ ] 实现 Telegram WebApp `initData` 校验。
- [ ] 实现 owner 白名单中间件。
- [ ] 实现 `bootstrap / open / close / kill / project-add / project-remove` API。
- [ ] 如有必要，在 `session.Registry` 增加只读状态聚合方法，避免 handler 自己拼状态。

### Task 4: 装配路由并接入静态资源

**Files:**
- Modify: `cmd/imtty-bridge/main.go`
- Create: `internal/miniapp/static.go`
- Modify: `internal/telegram/http.go` 或相关路由装配位置

- [ ] 把 `/mini-app` 与 `/mini-app/api/*` 路由接入主 `http.ServeMux`。
- [ ] 静态资源先按最小可用方式接入，支持本地构建产物服务。
- [ ] 启动时若配置了 `IMTTY_MINI_APP_BASE_URL`，调用 Telegram API 设置 Menu Button。

### Task 5: 先写前端最小联调约束

**Files:**
- Create: `web/mini-app/package.json`
- Create: `web/mini-app/vite.config.ts`
- Create: `web/mini-app/tsconfig.json`
- Create: `web/mini-app/src/lib/api.ts`

- [ ] 建最小前端工程。
- [ ] 约定前端只消费 `bootstrap` 和 5 个动作接口。
- [ ] 确保不引入额外状态管理库和复杂路由。

### Task 6: 搭建 Mini App 单页

**Files:**
- Create: `web/mini-app/src/main.tsx`
- Create: `web/mini-app/src/App.tsx`
- Create: `web/mini-app/src/components/...`
- Create: `web/mini-app/src/index.css`
- Create: `web/mini-app/components.json`

- [ ] 用 Tailwind CSS + shadcn/ui + Radix UI 实现单页布局。
- [ ] 做 4 个区块：当前会话、会话列表、项目列表、添加项目。
- [ ] 危险动作用确认弹层。
- [ ] 所有动作成功后重新拉取 `bootstrap`。

### Task 7: 文档与运维同步

**Files:**
- Modify: `README.md`
- Modify: `docs/prd.md`
- Modify: `docs/mvp-architecture-runbook.md`

- [ ] 补充 Mini App 配置项、Menu Button 配置和 quick tunnel 域名变化后的 Mini App URL 更新动作。
- [ ] 明确 Mini App 与聊天窗口的职责分工。

### Task 8: 完整验证

**Files:**
- Test: `internal/config/config_test.go`
- Test: `internal/miniapp/auth_test.go`
- Test: `internal/miniapp/http_test.go`
- Test: `internal/telegram/bot_test.go`

- [ ] 运行 `GOCACHE=/tmp/imtty-go-build go test ./...`
- [ ] 运行前端构建命令，确认静态产物可生成。
- [ ] 启动 bridge，确认 `/healthz`、`/mini-app`、`/mini-app/api/bootstrap` 路由可用。
