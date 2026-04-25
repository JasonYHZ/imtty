# imtty App Server Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用 Codex app-server 结构化事件替换 stdout pump，保留 tmux 承载层，并加入本地 attach 互斥控制。

**Architecture:** tmux session 内运行 `codex app-server`，bridge 通过 WebSocket 连接 app-server，使用 `thread/start|resume` 和 `turn/start` 驱动交互。Telegram 只发送 `final_answer`、审批请求和必要状态，普通 pane scraping 从主链路移除。

**Tech Stack:** Go, tmux, Codex app-server JSON-RPC over WebSocket, Telegram Bot API, Tailwind/shadcn Mini App existing API layer

---

### Task 1: 更新主链路文档

**Files:**
- Create: `docs/superpowers/specs/2026-04-25-imtty-app-server-rewrite-design.md`
- Create: `docs/superpowers/plans/2026-04-25-imtty-app-server-rewrite.md`
- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `docs/prd.md`
- Modify: `docs/mvp-architecture-runbook.md`

- [ ] **Step 1: 把主链路从 stdout pump 改写为 app-server 结构化事件**

要求写明：

```text
tmux session 内运行 codex app-server
Telegram 默认只消费 final_answer / 审批 / 必要状态
stdout pump 不再是默认路径
本地 attach 时 Telegram 不允许写入或 kill
```

- [ ] **Step 2: 自查文档是否还有 stdout pump 作为默认链路的表述**

Run: `rg -n "stdout pump|capture-pane|tmux capture" README.md AGENTS.md docs/prd.md docs/mvp-architecture-runbook.md`
Expected: 只允许出现“已移除”或“历史对比”语境，不能再出现“默认主链路”表述

### Task 2: 先写 tmux 元数据与互斥检测测试

**Files:**
- Modify: `internal/tmux/manager_test.go`
- Modify: `internal/tmux/manager.go`

- [ ] **Step 1: 先写失败测试，覆盖 tmux option 元数据与 attached client 检测**

测试目标：

```go
func TestManagerEnsureSessionStoresAppServerPort(t *testing.T) {}
func TestManagerSessionMetadataRoundTrip(t *testing.T) {}
func TestManagerHasAttachedClients(t *testing.T) {}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/tmux`
Expected: FAIL，提示缺少端口元数据写入或 attached client 检测实现

- [ ] **Step 3: 最小实现 tmux 元数据读写与本地 attach 检测**

实现点：

```go
func (m *Manager) EnsureSession(...) (SessionRuntimeInfo, error)
func (m *Manager) SessionMetadata(...) (SessionRuntimeInfo, error)
func (m *Manager) SetThreadID(...) error
func (m *Manager) HasAttachedClients(...) (bool, error)
```

- [ ] **Step 4: 再跑 tmux 测试**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/tmux`
Expected: PASS

### Task 3: 先写 app-server 客户端测试

**Files:**
- Create: `internal/appserver/client.go`
- Create: `internal/appserver/client_test.go`
- Create: `internal/appserver/types.go`

- [ ] **Step 1: 先写失败测试，覆盖 initialize、resume/start、final_answer、approval**

测试目标：

```go
func TestClientConnectAndStartThread(t *testing.T) {}
func TestClientResumeThreadWhenThreadIDExists(t *testing.T) {}
func TestClientEmitsFinalAnswerOnly(t *testing.T) {}
func TestClientTracksPendingApprovalRequests(t *testing.T) {}
func TestClientRespondsToApprovalDecision(t *testing.T) {}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/appserver`
Expected: FAIL，提示包或类型不存在

- [ ] **Step 3: 最小实现 WebSocket JSON-RPC 客户端**

实现点：

```go
type Event struct { ... }
type ApprovalRequest struct { ... }
type Client struct { ... }

func NewClient(...) *Client
func (c *Client) Connect(ctx context.Context) error
func (c *Client) EnsureThread(ctx context.Context, threadID string, cwd string) (string, error)
func (c *Client) StartTurn(ctx context.Context, text string) error
func (c *Client) Approve(ctx context.Context, decision ApprovalDecision) error
func (c *Client) Events() <-chan Event
func (c *Client) Close() error
```

- [ ] **Step 4: 再跑 app-server 测试**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/appserver`
Expected: PASS

### Task 4: 先写 Telegram 新行为测试

**Files:**
- Modify: `internal/telegram/adapter_test.go`
- Modify: `internal/miniapp/http_test.go`
- Modify: `internal/telegram/adapter.go`

- [ ] **Step 1: 先写失败测试，锁定新语义**

测试目标：

```go
func TestWebhookHandlerPlainTextStartsTurnWithoutImmediateAck(t *testing.T) {}
func TestWebhookHandlerRejectsPlainTextWhenSessionLocallyAttached(t *testing.T) {}
func TestWebhookHandlerRoutesApprovalReplyToPendingRequest(t *testing.T) {}
func TestWebhookHandlerRejectsKillWhenSessionLocallyAttached(t *testing.T) {}
```

- [ ] **Step 2: 运行 telegram 相关测试确认失败**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/telegram ./internal/miniapp`
Expected: FAIL，提示仍依赖 OutputPump 或缺少互斥与审批路由

- [ ] **Step 3: 最小实现新的 Adapter 依赖接口**

接口方向：

```go
type SessionRuntime interface {
    OpenSession(ctx context.Context, chatID int64, session session.View) error
    CloseSession(sessionName string)
    SubmitText(ctx context.Context, chatID int64, session session.View, text string) error
    HasPendingApproval(sessionName string) bool
    SubmitApproval(ctx context.Context, session session.View, text string) (bool, error)
    IsLocallyAttached(ctx context.Context, sessionName string) (bool, error)
    KillSession(ctx context.Context, session session.View) error
}
```

- [ ] **Step 4: 再跑 telegram 相关测试**

Run: `GOCACHE=/tmp/imtty-go-build go test ./internal/telegram ./internal/miniapp`
Expected: PASS

### Task 5: 主进程装配切换并移除 stdout pump 接线

**Files:**
- Modify: `cmd/imtty-bridge/main.go`
- Modify: `cmd/imtty-bridge/reattach.go`
- Modify: `internal/session/registry.go`
- Delete: `internal/stream/pump.go`
- Delete: `internal/stream/pump_test.go`

- [ ] **Step 1: 先写失败检查，确认 main 仍在引用旧接线**

Run: `rg -n "NewPump|OutputPump|CaptureOutput" cmd internal`
Expected: 能看到旧接线，作为后续清理基线

- [ ] **Step 2: 实现新的主装配**

要求：

```text
main.go 不再实例化 stream.NewPump
adapter 注入新的 session runtime
reattach 只恢复 detached session 视图，不恢复 pane watcher
```

- [ ] **Step 3: 删除旧 pump 文件并修正剩余引用**

Run: `rg -n "NewPump|OutputPump|CaptureOutput|capture-pane" cmd internal`
Expected: 不再有主链路引用；如果仍保留测试或历史注释，必须是非主链路语境

- [ ] **Step 4: 跑全量 Go 测试**

Run: `GOCACHE=/tmp/imtty-go-build go test ./...`
Expected: PASS

### Task 6: 真实运行验证

**Files:**
- Modify: `config.toml.example`
- Modify: `README.md`

- [ ] **Step 1: 补充 app-server 重写后的运行说明**

要求写明：

```text
tmux session 内运行 codex app-server
本地 attach 互斥
bridge 重启后如何 resume thread
```

- [ ] **Step 2: 重启 bridge 并检查健康状态**

Run: `kill <old-pid>`
Run: `GOCACHE=/tmp/imtty-go-build go run ./cmd/imtty-bridge`
Run: `curl -s http://127.0.0.1:8080/healthz`
Expected: `ok`

- [ ] **Step 3: 做一轮 Telegram 真实联调**

验证项：

```text
/open imtty
发送 hi
只收到 final_answer
触发一个审批请求并确认 Telegram 有明确审批提示
本地 attach 后再次发送文本，收到“本地占用中”提示
```
