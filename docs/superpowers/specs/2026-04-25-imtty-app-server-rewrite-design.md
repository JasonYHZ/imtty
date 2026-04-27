# imtty App Server Rewrite Design

## Summary

本次重写把 `imtty` 的 Telegram 输出主链路从 `tmux capture-pane/stdout pump` 改为 `Codex app-server` 的结构化事件流。`tmux` 仍然是唯一会话承载层，但它只负责托管长期运行的 Codex app-server 进程，不再承担 Telegram 默认输出源。

目标是解决当前 pane scraping 架构的根问题：

- Telegram 聊天窗口看到的是终端屏幕状态，而不是结构化会话事件。
- prompt、过程态、工具轨迹和最终回复混在一起，过滤规则会不断失效。
- 本地 attach 与 Telegram 同时写入同一会话时，输入提交与输出顺序都不稳定。

## Scope

本次重写只覆盖 Telegram 主链路和 bridge 内部会话运行时：

- 保留现有 bot 命令面与 Mini App 命令等价语义。
- 保留 `tmux` 作为唯一 session carrier。
- 删除 stdout pump 在默认链路中的接线。
- 新增 `internal/appserver/` 作为结构化事件客户端。
- 新增本地 attach 互斥规则。

不进入本次范围：

- 多用户
- 新 IM adapter
- Web 控制台
- 数据库
- Telegram 原始终端镜像模式

## Architecture

### Session process model

每个 `codex-{project}` tmux session 内启动的主进程从 `codex` TUI 改为：

`codex app-server --listen ws://127.0.0.1:<port>`

bridge 通过 WebSocket 连接这个 app-server，发送：

- `initialize`
- `thread/start` 或 `thread/resume`
- `turn/start`

接收：

- `item/completed` 中的 `agentMessage`
- `item/commandExecution/requestApproval`
- `item/fileChange/requestApproval`
- `item/permissions/requestApproval`
- `turn/completed`

### Structured output model

Telegram 默认只消费这几类结构化事件：

- `agentMessage` 且 `phase == final_answer`
- 审批请求
- turn 错误或 app-server 连接断开提示
- 必要状态提示

Telegram 默认不再消费：

- commentary
- reasoning
- command output delta
- file change delta
- Codex 内部维护任务报告，例如 memory 更新报告
- prompt
- terminal status

### Thread continuity

bridge 为每个 tmux session 持久化最小元数据到 tmux session options：

- `@imtty_port`
- `@imtty_thread_id`

这样 bridge 重启后可以：

1. 通过 `tmux list-sessions` 恢复 `codex-*` session
2. 通过 `@imtty_port` 重新连接已有 app-server
3. 优先用 `@imtty_thread_id` 调 `thread/resume`
4. 如果没有 thread id，再按 `cwd` 查找最近线程并恢复

### Local attach mutual exclusion

新增硬规则：

- 当 `tmux list-clients -t codex-{project}` 发现有本地可写 attach client 时，Telegram 不允许再向该 session 写入新 turn，也不允许远程 kill。
- 当本地仅有只读 spectator client 时，Telegram 仍允许继续远程写入和远程 kill。
- 允许的只读动作仍包括 `/list`、`/projects`、`/status`。
- `/open` 可以绑定该 session，但若存在可写 attach，后续文本输入和审批回复会被拒绝，并提示“本地占用中，请先在桌面端 detach 后再继续远程操作”。

## Component changes

### `internal/tmux/`

新增职责：

- 为新 session 分配 app-server 监听端口
- 在 tmux session options 中读写 `@imtty_port` / `@imtty_thread_id`
- 检测当前 session 是否存在本地 attach client

删除默认职责：

- 默认输出抓取
- `capture-pane` 作为 Telegram 主链路输入

### `internal/appserver/`

新增包，负责：

- JSON-RPC over WebSocket 连接
- initialize / thread start-resume / turn start
- 结构化通知分发
- 审批请求挂起与响应
- final answer 分片后发送到 Telegram

### `internal/telegram/`

Adapter 改为依赖“会话运行时”而不是 `OutputPump`：

- `/open` 负责确保 tmux + app-server session 可用，并把 active session 绑定到当前 chat
- 普通文本改为提交 `turn/start`
- 审批快捷回复改为响应挂起的 app-server approval request
- 本地 attach 冲突时返回中文状态提示

### `internal/stream/`

保留：

- 消息分片
- Telegram 友好文本格式化
- 审批快捷回复文案映射

删除默认职责：

- `Pump`
- pane diff
- quiet window
- redraw 去重

## Error handling

- app-server 连接失败：提示“会话未就绪，请稍后重试或重新 /open <project>”
- `thread/resume` 失败：回退到 `thread/start`
- turn 运行中断：停止 Telegram typing，并把 app-server 错误文本作为状态提示发回 Telegram。
- 审批请求悬挂期间收到无关普通文本：提示“当前等待审批，请先回复 是/否”
- 本地 attach 冲突：提示“本地占用中，请先在桌面端 detach 后再继续远程操作”
- app-server 连接断开但 tmux 仍存在：停止 Telegram typing，提示重新 `/open <project>` 或 `/status` 查看状态。

## Acceptance criteria

1. `/open project-a` 会创建或绑定 `codex-project-a`，并在 tmux 内运行 Codex app-server，而不是依赖 stdout pump。
2. 普通文本只会把 `final_answer` 发到 Telegram，不会出现过程态、工具轨迹或旧 pane 尾巴。
3. 当 app-server 发起 command/file/permissions 审批时，Telegram 会收到明确审批提示和 `是/否` 快捷回复。
4. 用户回复 `是` 或 `否` 时，bridge 会解析为结构化审批响应，而不是把原始文本再当成普通 turn 输入。
5. 当本地 attach 存在时，Telegram 普通文本输入与远程 kill 会被拒绝，并得到下一步动作提示。
6. bridge 重启后，已有 `codex-*` tmux session 能被重新发现，并通过已记录的 thread id 或最近 thread 继续会话。
7. 主链路中不再实例化或启动 stdout pump。
