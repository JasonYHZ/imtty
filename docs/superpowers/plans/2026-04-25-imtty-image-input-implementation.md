# imtty Image Input Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 支持 Telegram `photo` 和图片型 `document` 作为临时本地图片输入发给 Codex。

**Architecture:** Telegram webhook 解析图片消息，通过 `getFile` 和文件下载拿到原始图片，保存到临时目录，再由 app-server `turn/start` 以 `localImage + text` 形式提交。

**Tech Stack:** Go, Telegram Bot API `getFile`, local temp files, Codex app-server `localImage`

---

### Task 1: 更新图片输入文档

**Files:**
- Create: `docs/superpowers/specs/2026-04-25-imtty-image-input-design.md`
- Create: `docs/superpowers/plans/2026-04-25-imtty-image-input-implementation.md`
- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `docs/prd.md`
- Modify: `docs/mvp-architecture-runbook.md`

- [ ] **Step 1: 文档中正式打开“最小图片输入支持”边界**

要求写明：

```text
支持 photo 和图片型 document
下载到临时目录
通过 localImage 进入 Codex
不支持多图、视频、长期留档
```

- [ ] **Step 2: 自查旧文档里“不支持图片”的硬排除是否已改成新边界**

Run: `rg -n "不做文件上传、图片|图片消息|document" README.md AGENTS.md docs/prd.md docs/mvp-architecture-runbook.md`
Expected: 不再把单张图片输入列为绝对禁止项

### Task 2: 先写 Telegram 文件下载与临时存储测试

**Files:**
- Create: `internal/media/store.go`
- Create: `internal/media/store_test.go`
- Modify: `internal/telegram/bot.go`
- Modify: `internal/telegram/bot_test.go`

- [ ] **Step 1: 先写失败测试**

测试目标：

```go
func TestBotClientGetFileReadsTelegramFilePath(t *testing.T) {}
func TestBotClientDownloadFileFetchesBinaryBody(t *testing.T) {}
func TestStoreSaveImageWritesTempFileAndCleansExpiredFiles(t *testing.T) {}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/telegram ./internal/media`
Expected: FAIL，提示缺少 getFile / 下载 / store 实现

- [ ] **Step 3: 写最小实现**

要求：

```text
BotClient 增加 GetFile 和 DownloadFile
media.Store 默认写到 os.TempDir()/imtty-media
写新文件前顺手清理 24h 前旧文件
```

- [ ] **Step 4: 再跑测试**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/telegram ./internal/media`
Expected: PASS

### Task 3: 先写 app-server 多输入 turn 测试

**Files:**
- Modify: `internal/appserver/client.go`
- Modify: `internal/appserver/client_test.go`
- Modify: `internal/appserver/runtime.go`

- [ ] **Step 1: 先写失败测试**

测试目标：

```go
func TestClientStartTurnWithLocalImageAndCaption(t *testing.T) {}
func TestRuntimeSubmitImageUsesLocalImageInput(t *testing.T) {}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/appserver`
Expected: FAIL，提示缺少多输入 turn 或 SubmitImage

- [ ] **Step 3: 写最小实现**

要求：

```text
Client 支持 StartTurnInputs
Runtime 增加 SubmitImage
图片输入使用 localImage，caption 用 text
```

- [ ] **Step 4: 再跑测试**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/appserver`
Expected: PASS

### Task 4: 先写 Telegram 图片消息测试

**Files:**
- Modify: `internal/telegram/types.go`
- Modify: `internal/telegram/adapter.go`
- Modify: `internal/telegram/adapter_test.go`

- [ ] **Step 1: 先写失败测试**

测试目标：

```go
func TestWebhookHandlerPhotoMessageSubmitsImageTurn(t *testing.T) {}
func TestWebhookHandlerImageDocumentSubmitsImageTurn(t *testing.T) {}
func TestWebhookHandlerRejectsNonImageDocument(t *testing.T) {}
func TestWebhookHandlerImageMessageRequiresActiveSession(t *testing.T) {}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/telegram`
Expected: FAIL，提示 Message 结构或图片分支缺失

- [ ] **Step 3: 写最小实现**

要求：

```text
Message 支持 caption / photo[] / document
Adapter 在 plain text 之前识别图片消息
photo 取最大尺寸
非图片 document 直接拒绝
```

- [ ] **Step 4: 再跑 telegram 测试**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/telegram`
Expected: PASS

### Task 5: 主进程接线与全量验证

**Files:**
- Modify: `cmd/imtty-bridge/main.go`
- Modify: `README.md`

- [ ] **Step 1: 把 file client 和 media store 接到 Adapter**

要求：

```text
main.go 创建 BotClient 后同时把它作为 Telegram file client 注入
main.go 创建 media.Store 并注入 Adapter
```

- [ ] **Step 2: 跑全量测试**

Run: `GOCACHE=/tmp/imtty-go-build go test ./...`
Expected: PASS

- [ ] **Step 3: 重启 bridge 并做最小运行验证**

Run: `curl -s http://127.0.0.1:8080/healthz`
Expected: `ok`

验证项：

```text
/open imtty
发一张 photo
发一张图片型 document
发一个非图片 document，确认被拒绝
```
