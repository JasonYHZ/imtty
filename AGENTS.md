# imtty Guardrails

本文件是未来实现 `imtty` 时必须遵守的强约束，不是导览页。

## 产品与范围硬边界

- MVP 只服务个人单机场景，不做多用户设计。
- MVP 只覆盖 `IM -> Bridge -> tmux/PTY -> Codex`。
- Telegram adapter 是唯一优先实现的 IM 入口；其他 IM 不在当前范围。
- Telegram Mini App 允许作为 Telegram 内部的轻量 companion UI，但不能演化成独立后台系统。
- 不实现 Agent 编排、任务拆解、自主调度、自主重试策略。
- 不实现独立 Web 面板、后台管理界面或额外运维面板。
- 不实现复杂权限系统、角色系统或共享工作空间模型。
- 不定义通用文件上传、语音、富卡片、批量任务控制。
- 允许最小图片输入支持：Telegram `photo` 与图片型 `document` 可以作为单张临时图片输入进入 Codex。
- 允许最小文件分析支持：Telegram 文本/代码类 `document` 与 `PDF document` 可以作为临时文件输入进入 Codex。
- 不改写 Codex 的审批协议；必须保留 Codex 原生 REPL/approval 流。

## 默认实现边界

- 语言和服务形态默认是 `Go` 单服务。
- `tmux` 是唯一会话承载层；不要引入 screen、原生 PTY 池或其他 session backend 作为 MVP 主路径。
- Codex 进程必须跑在 `tmux session` 内；默认运行形态是 `codex app-server`，由 bridge 负责连接其结构化事件流。
- Telegram 默认输出源必须是 app-server 的结构化事件，不能再以 `tmux capture-pane/stdout pump` 作为主链路。
- 对用户可见的输出仍必须做 Telegram 友好分片和节流，不能把原始终端垃圾直接发到 IM。
- 普通文本消息默认透传到当前 active session。
- 单张图片消息允许通过临时文件 + `localImage` 进入当前 active session。
- 文本/代码文件与 PDF 允许通过临时文件 + 内容提取进入当前 active session。
- `Yes/No` 快捷回复只是便捷层，最终仍要回注为文本输入。
- 单个 IM 会话在任一时刻只能绑定一个 active session；允许存在多个 project session。
- Mini App 只做结构化控制，不替代聊天窗口中的 stdout、普通文本输入或审批流。
- 当本地终端已 attach 到某个 `codex-{project}` session 时，Telegram 不允许继续向该 session 写入普通文本、审批回复或 kill 动作。

## 文档优先规则

- 任何新增命令、状态、交互语义、恢复策略，先改文档再改代码。
- 如果实现与文档冲突，以文档为准，先修正文档或回退实现，不允许默默漂移。
- 如果未来要扩大范围，先在 `docs/` 明确新增边界，再讨论代码结构。

## 实现约束

- 项目名对应的 session 命名固定为 `codex-{project}`。
- 会话状态至少支持：`idle`、`starting`、`running`、`detached`、`exited`、`lost`。
- 对外 bot 命令面只保留：`/list`、`/projects`、`/project_add <name> <abs-path>`、`/project_remove <name>`、`/open <project>`、`/close`、`/kill`、`/clear`、`/status`、`/model [model]`、`/reasoning [effort]`、`/plan_mode [default|plan]`。
- `/restart` 不是 MVP bot 命令；如需恢复增强，只能先作为后续占位写在文档中。
- Bridge 重启后必须优先尝试接管已有 `tmux session`，而不是默认重启 Codex。
- 任何状态提示都要告诉用户当前状态和下一步动作，不允许只报技术错误。

## 建议目录边界

- `cmd/imtty-bridge/`: 进程入口与启动装配
- `internal/telegram/`: Telegram webhook adapter 和消息出入站
- `internal/session/`: active session、registry、状态机
- `internal/tmux/`: tmux session 管理和命令调用
- `internal/appserver/`: Codex app-server 客户端、事件解析、审批响应
- `internal/stream/`: 文本分片、节流、审批快捷回复文案
- `internal/config/`: 环境变量与项目根目录配置
- `internal/miniapp/`: Mini App 鉴权、API 与静态资源挂载
- `web/mini-app/`: Mini App 前端单页
- `docs/`: 产品、架构、操作手册

## 禁止事项

- 不要在 MVP 中引入数据库作为前提。
- 不要让 bridge 在后台替用户自动确认审批。
- 不要在未定义项目白名单时允许任意路径打开。
- 不要用“智能路由”同时把一条用户消息发给多个 session。
- 不要为了方便调试破坏 Telegram-first 的公开接口定义。
- 不要把 Telegram 图片长期保存到项目目录或仓库目录。
- 不要把 Telegram 文件长期保存到项目目录或仓库目录。
