# imtty PRD

## 1. 背景

个人在外出或离开桌面时，仍希望通过 IM 远程接入本机上的 Codex 会话，继续查看输出、发送文本、处理审批，并在必要时恢复已有终端上下文。现有远程方案通常要么过于底层，只暴露原始 shell；要么过于自动化，引入 Agent 编排、自主调度或多用户系统复杂度，不适合个人单机、人工主导的工作方式。

`imtty` 的目标是提供一个最小桥接层，把 Telegram 与本机 `tmux + Codex` 稳定连接起来，让用户能在 IM 中继续操作当前工作，而不改变 Codex 原生交互语义。

## 2. 产品目标

- 提供 Telegram 到本机 Codex session 的稳定文本桥接。
- 保持 human-in-the-loop，所有关键动作仍由用户明确输入。
- 支持多个 project session 并存，但始终只有一个 active session 绑定当前 IM 会话。
- 在 bridge 或网络抖动后，可重新接管既有 `tmux session`，减少上下文丢失。
- 让远程交互保留 Codex 会话语义，但 Telegram 只消费结构化最终回复、审批和必要状态，而不是终端屏幕状态。

## 3. 非目标

- 不做 Agent 编排、任务拆解、自主调度、自主重试。
- 不做多用户权限系统、团队共享或协作审计。
- 不做 Web UI、控制台后台、独立 dashboard。
- 不做通用文件上传、语音、富卡片或复杂多模态交互。
- 允许最小图片输入支持：Telegram `photo` 和图片型 `document` 作为单张临时图片输入进入 Codex。
- 允许最小文件分析支持：Telegram 文本/代码类 `document` 和 `PDF document` 作为临时文件输入进入 Codex。
- 不做对 Codex 协议的再包装或替代，不屏蔽 Codex 的原生审批流程。

## 4. 核心原则

- Telegram-first：MVP 的唯一公开入口是 Telegram Bot。
- Telegram Mini App 可以作为 Telegram 内部的轻量 companion UI，但不替代聊天窗口。
- Single-user：默认只有一个真实使用者，配置和恢复流程围绕单人本机场景设计。
- Session-first：桥接的是项目会话，不是无状态命令调用。
- Human-in-the-loop：所有执行、审批和恢复动作都由用户发起。
- Document-first：交互面、状态机和恢复语义先在文档中定，再落代码。

## 5. 目标用户与场景

目标用户是个人开发者本人。典型场景：

- 离开桌面但需要继续向 Codex 补充指令。
- 需要在手机上看到 Codex 输出并处理 `y/n` 风格审批。
- Bridge 进程重启后，希望继续接管先前项目的 tmux 上下文。
- 同时维护多个项目，但在 Telegram 中只操作当前 active project。

## 6. 用户流程

### 6.1 打开项目会话

1. 用户发送 `/open project-a`。
2. Bridge 校验 `project-a` 是否在允许列表中。
3. Bridge 创建或绑定命名为 `codex-project-a` 的 `tmux session`。
4. 如果 session 中尚未运行 Codex，则在该项目根目录中启动 Codex。
5. Bridge 将当前 IM 会话的 active session 切换到 `project-a`。
6. 用户收到状态提示，可开始发送普通文本。

### 6.2 普通交互

1. 用户发送普通文本。
2. Bridge 将文本注入当前 active session。
3. bridge 通过 Codex app-server 接收结构化事件。
4. 只有最终回复、审批请求和必要状态会被格式化并发回 Telegram。

### 6.3 审批交互

1. Codex 输出中出现 `y/n`、`Yes/No` 等审批提示。
2. Bridge 检测到审批迹象，可在 Telegram 侧附带 `Yes` / `No` 快捷回复。
3. 用户点击快捷回复或手动输入文本。
4. Bridge 统一把用户选择回注为文本输入。
5. 文本回注是唯一真相源；快捷回复只是输入便利层。

### 6.4 关闭与终止

- `/close`：解绑当前 active session，不强杀底层 `tmux session`。
- `/kill`：终止当前 active session 对应的 Codex/tmux 会话，并彻底删除该会话记录。
- `/clear`：为当前 active session 立即创建一个新的空 thread，但保留底层 tmux/Codex app-server 进程。
- `/status`：返回当前 active session 的详细状态、当前 Codex 窗口统计和建议下一步动作。
- Mini App：提供结构化按钮、列表和表单，用于触发与上述命令等价的控制动作。

## 7. 对外命令面

MVP 只定义以下 bot 命令：

- `/list`: 只列出已知 session 的简要状态。
- `/projects`: 列出允许打开的项目。
- `/project_add <name> <abs-path>`: 动态添加一个可打开项目，并写入本地持久化白名单。
- `/project_remove <name>`: 删除一个通过聊天窗口动态加入的项目。
- `/open <project>`: 创建或绑定该项目的 session，并将其设为 active。
- `/open <project> <thread-id>`: 只在该项目当前 thread 与给定 thread id 精确匹配时才绑定该 session。
- `/close`: 关闭当前 IM 绑定，不销毁底层 session。
- `/kill`: 杀掉当前 active session 对应的 tmux/Codex 会话，并从已知会话列表中移除。
- `/clear`: 立即清空当前 active session 的对话 thread，并切换到新的 thread id。
- `/status`: 查看当前 active session 的详细状态与窗口统计。
- `/model [model-id]`: 查看或设置当前 active session 的待生效模型。
- `/reasoning [effort]`: 查看或设置当前 active session 的待生效 reasoning。
- `/plan_mode [default|plan]`: 查看或设置当前 active session 的待生效 plan 预设。

MVP 对外输入模型只允许五类：

- bot command
- plain text
- quick reply approval
- single image message
- single file message

MVP 不定义：

- `/restart` bot 命令
- 任意文件上传
- 语音消息
- 富卡片式交互
- 独立 Web 控制台

## 8. 会话模型

### 8.1 命名规则

- 统一命名为 `codex-{project}`。

### 8.2 绑定规则

- 单用户下可并存多个 project session。
- 任一时刻只有一个 active session 绑定到当前 IM 会话。
- `/open` 会切换 active session。
- `/open <project> <thread-id>` 失败时不应破坏当前已有 active session 绑定。
- `/close` 只解除绑定，不销毁后台 session。
- `/kill` 会删除该 project 的现有 session 记录；后续需要重新 `/open` 才会重新创建。
- `/clear` 只重置当前 active session 的 thread，不重建 tmux session，也不切换 active 绑定。
- `/model`、`/reasoning`、`/plan_mode` 只作用于当前 active session。
- 这些动态控制不会伪造空 turn；它们会在下一条真实用户输入时随 `turn/start` 一起生效。

### 8.3 状态定义

- `idle`: session 已知但当前无运行中的 Codex 交互。
- `starting`: bridge 正在创建 session 或启动 Codex。
- `running`: session 正常运行并可收发消息。
- `detached`: session 仍存在，但当前 IM 未绑定或 bridge 未附着流。
- `exited`: Codex 已退出，session 可见但不再活跃。
- `lost`: 预期中的 tmux session 无法找到或状态不可信。

## 9. 输入与输出链路

### 9.1 输入链路

`Telegram message -> webhook adapter -> command router -> app-server turn/start 或 approval response -> Codex`

图片输入补充链路：

`Telegram photo/document -> getFile/download -> temp file -> app-server localImage -> Codex`

文件输入补充链路：

`Telegram document -> getFile/download -> temp file -> 文本读取或 PDF 文本提取 -> app-server text -> Codex`

### 9.2 输出链路

`Codex app-server events -> output formatter -> chunker -> Telegram sender`

### 9.3 输出规则

- 必须按 Telegram 消息体限制分片。
- 必须保持同一 turn 内最终回复与审批提示的顺序。
- 必须在短时间连续输出时做节流，避免发信风暴。
- Telegram 默认只发送最终 assistant 回复、审批提示和必要状态，不镜像原始终端过程流。
- 当 bridge 已成功把 turn 提交给 Codex 后，Telegram 可显示原生 `typing` 状态，直到最终回复或审批提示返回。
- commentary、工具调用轨迹、terminal 输出、prompt、状态栏、内部推理和类似 TUI 过程噪音默认不得发到 Telegram。
- Codex 内部维护任务的最终报告不得发到 Telegram，例如 memory 更新报告。
- 普通文本成功注入当前 active session 后默认不发送确认回执，避免和短最终回复形成视觉重复。
- `/status` 追求 app-server 协议支持下的最佳努力，不承诺与 Codex TUI 状态行逐字段完全一致。
- 出错时要保留最小可读上下文，不能只发“发送失败”。
- 当本地终端存在可写 attach client 时，Telegram 不允许继续写入该会话。
- 当本地终端仅存在只读 spectator client 时，Telegram 仍允许继续远程写入该会话。
- 图片只允许作为临时文件落地，不写入项目目录，也不长期保存。
- 文本/代码文件与 PDF 只允许作为临时文件落地，不写入项目目录，也不长期保存。

## 10. 异常处理

- 打开未知项目：返回拒绝信息和允许的项目范围。
- 无 active session 时收到普通文本：明确提示先执行 `/open <project>`。
- Bridge 重启：重建 session registry，并优先恢复已有 tmux session 对应的 app-server thread。
- Codex 退出：将 session 状态标为 `exited`，提示用户重新 `/open` 或等待后续增强。
- tmux session 丢失：将状态标为 `lost`，提示用户重新 `/open <project>` 创建。
- Telegram 发送失败：记录失败并继续后续队列处理，避免阻塞后续结构化事件发送。

## 11. 安全模型

- 部署在用户本人控制的单机环境。
- 项目根目录必须来自环境变量静态白名单，或由用户通过 `/project_add` 显式加入本地白名单，而不是任意路径直接透传到 `/open`。
- Telegram 图片下载后只允许写入 bridge 主机临时目录，不能借机突破项目白名单或写入项目目录。
- Telegram 文件下载后只允许写入 bridge 主机临时目录，不能借机突破项目白名单或写入项目目录。
- Mini App 如果提供目录浏览能力，必须明确区分“浏览 bridge 主机目录”和“真正加入项目白名单”两个动作；前者允许用户在 bridge 主机上做系统级目录选择，后者仍然必须通过显式确认写入白名单。
- 通过 `IMTTY_TELEGRAM_WEBHOOK_SECRET` 校验 webhook 来源。
- Mini App API 必须通过 Telegram `initData` 与 owner 白名单校验，不能复用 webhook secret。
- Bot token 只通过环境变量配置，不写入代码或文档示例中的真实值。
- 不在 bridge 中自动批准任何 Codex 审批。
- 不把快捷回复当成独立审批协议；它只是普通文本输入包装。

## 12. MVP 范围

MVP 必须包含：

- Telegram webhook 接入
- Telegram Mini App companion UI
- 项目白名单与 `/projects`
- `/project_add`、`/project_remove`
- `/open`、`/close`、`/kill`、`/clear`、`/status`
- `tmux` session 创建、绑定、探测、销毁
- 普通文本透传
- 单张图片输入透传
- 文本/代码文件与 PDF 的临时文件输入透传
- Codex app-server 连接、thread start-resume、turn start
- 结构化最终回复分片与节流
- 结构化审批请求与 `Yes/No` 快捷回复映射
- bridge 重启后的 session reattach

MVP 明确不包含：

- `/restart` bot 命令
- 多用户账号体系
- Web 管理面板
- 任意文件上传和扫描版 PDF OCR
- 后台任务编排

## 13. 后续范围

后续可以考虑，但不进入 MVP：

- `/restart` 作为显式恢复命令
- 更细粒度的输出订阅与静默模式
- 更丰富的项目元数据展示
- 其他 IM adapter
- 轻量审计日志或操作回放

## 14. 验收标准

以下场景全部通过，MVP 才算满足目标：

1. `/open project-a` 能创建或绑定 `codex-project-a`，随后普通文本可进入 Codex。
2. Codex 最终回复能按 Telegram 限制分段发送，不丢顺序，不泄漏过程态。
3. 遇到 `y/n` 风格审批时，可映射出 `Yes/No` 快捷回复，但最终仍以文本回注为唯一真相源。
4. Bridge 重启后能重新接管已有 tmux session，不要求重启 Codex。
5. Codex 退出或 tmux 丢失时，IM 侧会收到明确状态提示和下一步恢复动作。
6. 单用户下可以并存多个 project session，但任一时刻只有一个 active session 绑定到当前 IM 会话。
7. `/kill` 后该 session 不再出现在 `/list` 中；如果当前没有 active session，`/status` 只会提示下一步动作。
8. `/projects` 只显示可打开项目，`/list` 不显示项目白名单。
9. `/project_add demo /abs/path` 后，`/projects` 能立即看到 `demo`，bridge 重启后仍然保留。
10. `/project_remove demo` 后，`demo` 会从 `/projects` 中消失，且 `/open demo` 会被拒绝。
11. Mini App 可以显示 active session、session 列表与 project 列表，并触发等价控制动作。
12. Mini App 添加项目时，可以通过系统级目录选择器从 `~` 开始浏览 bridge 主机目录，并一路返回到 `/`，而不是只能手填绝对路径。
13. 当桌面端已经 attach 到同一 tmux session 时，Telegram 普通文本、审批回复和 `/kill` 都会被拒绝，并得到明确下一步动作提示。
14. Telegram `photo` 和图片型 `document` 能以单张临时图片输入进入当前 active session。
15. Telegram 文本/代码类 `document` 和 `PDF document` 能以临时文件分析输入进入当前 active session。
16. 不支持的二进制 `document` 会被明确拒绝。
17. `/model`、`/reasoning`、`/plan_mode` 能更新当前 active session 的 pending 控制状态，并在下一条真实消息时生效。
18. `/status` 能输出当前 active session 的 model、reasoning、plan mode、thread id、cwd、branch 以及最佳努力窗口统计。
