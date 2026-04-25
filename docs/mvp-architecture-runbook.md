# imtty MVP Architecture Runbook

本文件同时承担架构说明和操作手册，避免在 MVP 阶段拆出更多文档。

## 1. 目标

用最小内部组件，把 Telegram bot 消息稳定桥接到本机 `tmux` 中运行的 Codex app-server，并在 bridge 重启或 session 波动后可恢复接管。

## 2. 固定技术路线

- Runtime: `Go`
- IM adapter: `Telegram Bot webhook`
- Session carrier: `tmux`
- Interactive engine: `Codex app-server`

MVP 不引入数据库、消息队列或第二套 session backend。

## 3. 内部组件与职责

### 3.1 Telegram webhook adapter

职责：

- 验证 webhook secret。
- 解析 Telegram update。
- 区分 bot command、plain text、quick reply approval、single image message、single file message。
- 把输入转成内部命令或文本事件。

边界：

- 不做复杂业务状态判断。
- 不做 session 生命周期管理。

### 3.2 Session registry

职责：

- 维护 project 到 session 的映射关系。
- 维护当前 IM 会话的 active session。
- 记录状态：`idle`、`starting`、`running`、`detached`、`exited`、`lost`。
- 在 bridge 启动时从现有 `tmux session` 重建可见状态。

边界：

- 只维护 bridge 可见的会话元信息，不替代 `tmux` 本身。

### 3.3 Tmux session manager

职责：

- 创建、查找、attach、kill `codex-{project}` session。
- 在指定 project root 中启动 Codex。
- 探测 session 是否仍存活。

边界：

- `tmux` 是唯一 session 真相源。
- 不在此层做 Telegram 消息格式处理。

### 3.4 App-server runtime

职责：

- 连接 tmux session 内运行的 `codex app-server`
- 完成 `initialize`
- 根据 thread id 执行 `thread/resume`，或在首次打开时执行 `thread/start`
- 发送 `turn/start`
- 监听 `final_answer`、审批请求和 turn 完成事件

边界：

- 不直接管理 Telegram 命令路由
- 不读取 pane 或终端屏幕内容

### 3.5 Stdin injector

职责：

- 将用户普通文本转换成 app-server `turn/start`。
- 将用户图片消息转换成 `localImage` + 可选 caption 的 app-server 输入。
- 将用户文本/代码文件或 PDF 转换成内容提取后的 app-server 文本输入。
- 当存在挂起审批时，将 `Yes/No` 快捷回复转换成结构化 approval response。

边界：

- 不解释文本业务含义。
- 不自动补全或改写用户指令。

### 3.6 Output formatter

职责：

- 只格式化结构化最终回复与审批提示。
- 按 `IMTTY_MESSAGE_CHUNK_BYTES` 分片。
- 按 `IMTTY_FLUSH_INTERVAL_MS` 节流和批量 flush。
- 保证同一 turn 内输出顺序不乱。

边界：

- 不决定 session 状态。

### 3.7 Approval adapter

职责：

- 把 app-server 的 command/file/permissions approval request 转成 Telegram 可读提示。
- 为 Telegram 侧附加 `Yes/No` 快捷回复。

边界：

- 只做提示，不做自动审批。
- 不篡改审批决策语义。

## 4. 最小配置面

以下环境变量构成 MVP 最小配置：

- `IMTTY_TELEGRAM_BOT_TOKEN`: Telegram bot token。
- `IMTTY_TELEGRAM_WEBHOOK_SECRET`: webhook 请求校验 secret。
- `IMTTY_PROJECT_ROOTS`: 允许打开的项目根目录列表。
- `IMTTY_TMUX_PREFIX`: tmux session 前缀，默认设计为 `codex-`。
- `IMTTY_CODEX_BIN`: Codex CLI 可执行文件路径，默认 `codex`。
- `IMTTY_MESSAGE_CHUNK_BYTES`: 单条消息最大分片字节数。
- `IMTTY_FLUSH_INTERVAL_MS`: 输出聚合和节流时间窗口。
- `IMTTY_PROJECT_STORE_PATH`: 动态项目白名单持久化文件路径；默认 `.imtty-projects.json`。
- `IMTTY_TELEGRAM_OWNER_ID`: Mini App 允许访问的 Telegram owner id。
- `IMTTY_MINI_APP_BASE_URL`: 用于生成 Mini App `Menu Button` 的公开基础地址。
- `project_browse_roots`: `config.toml` 中的额外快捷目录入口集合，用于 Mini App 的目录选择器。

默认加载策略：

- bridge 启动时默认读取当前工作目录下的 `config.toml`
- `-config /abs/path/config.toml` 可指定其他配置文件
- `IMTTY_*` 环境变量作为覆盖层，高于 `config.toml`
- `-listen` 作为启动参数覆盖层，高于 `config.toml.listen`

建议：

- `IMTTY_PROJECT_ROOTS` 应显式配置 project name 到绝对路径的映射，不接受任意目录透传。
- `IMTTY_TMUX_PREFIX` 虽可配置，但公开命名语义仍应保持 `codex-{project}`。
- `project_browse_roots` 建议只配置高频工作目录，作为 Mini App 快捷入口补充，而不是替代系统级目录浏览。

## 5. 标准数据流

### 5.1 `/open <project>`

1. Telegram webhook adapter 收到命令。
2. Session registry 校验项目是否在允许列表中。
3. Tmux session manager 查找 `codex-{project}`。
4. 若不存在，则创建 session 并在 project root 内启动 Codex app-server。
5. Tmux session manager 为该 session 分配或读取 app-server 监听端口。
6. App-server runtime 连接该 port，并执行 `thread/resume` 或 `thread/start`。
7. Session registry 更新状态为 `starting` 再转 `running`。
8. active session 切换到该 project。

### 5.1.1 `/project_add <name> <abs-path>`

1. Telegram webhook adapter 解析命令，并校验路径必须为绝对路径。
2. Session registry 将该项目加入允许列表。
3. dynamic project store 将该项目写入本地持久化文件。
4. `/projects` 立即可见该项目，后续 `/open <project>` 可以使用。

### 5.1.2 `/project_remove <name>`

1. Telegram webhook adapter 校验该项目是否属于动态白名单。
2. 若该项目存在会话记录，则先停止对应 pump 并终止底层 tmux session。
3. Session registry 删除该项目的允许项和会话记录。
4. dynamic project store 更新本地持久化文件。

### 5.2 普通文本消息

1. Telegram webhook adapter 判断为 plain text。
2. Session registry 解析当前 active session。
3. 如果当前有挂起审批，则把文本解释为结构化 approval response。
4. 否则 App-server runtime 向对应 thread 发出 `turn/start`。
5. App-server 事件流返回 `final_answer` 或审批请求。
6. Output formatter 分片、节流后发回 Telegram。

### 5.2.3 `/model`、`/reasoning`、`/plan_mode`

1. Telegram webhook adapter 校验当前存在 active session。
2. 读取当前 session 的 bridge 控制状态与 app-server 线程状态。
3. `/model` 通过 `model/list` 校验目标模型，并同步计算 reasoning 兼容性。
4. `/reasoning` 按当前目标模型支持列表校验目标 effort。
5. `/plan_mode` 在 bridge 内部映射成 `default` 或 `plan` 预设，不直接透传成 app-server 原生命令。
6. 命令只更新当前 session 的 pending 控制状态，不会伪造空 turn。
7. 下一条真实用户输入进入 `turn/start` 时，pending model / effort 一并覆盖到底层会话，并提升为 effective 状态。

### 5.2.1 图片消息

1. Telegram webhook adapter 识别 `photo` 或图片型 `document`。
2. 通过 Telegram `getFile` 获取 `file_path`。
3. 下载原始图片到 bridge 主机临时目录。
4. App-server runtime 以 `localImage` 输入提交图片，并附带可选 caption 文本。
5. 非图片 `document` 直接拒绝。

默认语义：

- Telegram 默认只接收最终 assistant 回复、审批提示和必要状态。
- commentary、工具执行轨迹、terminal 输出、prompt、状态栏、内部推理和类似 TUI 过程噪音默认不发到 Telegram。
- 普通文本成功送入当前 active session 后默认静默，不额外回一条“已发送到 ...”。
- 当 turn 已提交给 Codex 且仍在处理中时，bridge 会周期性发送 Telegram `typing` chat action，直到最终回复或审批提示出现。
- 本地 attach 时，普通文本与审批回复直接拒绝，不进入远程 turn。
- 图片只允许写入 bridge 主机临时目录，不长期保存，也不复制进项目目录。

### 5.2.2 文件消息

1. Telegram webhook adapter 识别文本/代码类 `document` 或 `PDF document`。
2. 通过 Telegram `getFile` 获取 `file_path`。
3. 下载原始文件到 bridge 主机临时目录。
4. 文本/代码类文件直接读取内容；PDF 由 bridge 本地提取文本。
5. 提取后的文本与可选 caption 组合成 app-server `text` 输入。
6. 不支持的二进制 `document` 直接拒绝。

默认语义补充：

- 文本/代码文件与 PDF 只允许写入 bridge 主机临时目录，不长期保存，也不复制进项目目录。
- 第一版不做扫描版 PDF OCR，只处理可直接提取文本的 PDF。

### 5.2.1 Mini App 控制动作

1. Telegram 内部通过 `Menu Button` 打开 Mini App。
2. 前端携带 `initData` 请求 `GET /mini-app/api/bootstrap`。
3. 服务端验证 `initData` 与 owner id。
4. `bootstrap` 同时返回 project 列表、目录选择器默认路径和快捷入口列表。
5. Mini App 直接以 bridge 主机绝对路径请求目录浏览接口，支持返回父目录并一路浏览到 `/`。
6. Mini App 通过结构化接口触发 `open / close / kill / project-add / project-remove`。
7. 这些动作必须复用现有 bridge 语义，而不是重新实现一套 session 生命周期。

### 5.3 `/close`

1. 解除当前 IM 会话与 active session 的绑定。
2. session 状态转为 `detached` 或保留其当前运行态的可见标记。
3. 不杀掉底层 tmux/Codex。

### 5.4 `/kill`

1. 查找当前 active session。
2. 若当前 session 有本地可写 attach client，则拒绝远程 kill 并提示先在本地 detach。
3. 否则终止底层 tmux session 或其内部 Codex app-server 进程。
4. Session registry 删除该 session 记录。
5. 给用户返回明确结果和恢复动作。

### 5.5 `/clear`

1. 只作用于当前 active session。
2. bridge 不重建 `tmux session`，也不重启 `codex app-server`。
3. bridge 立即向 app-server 创建一个新的 thread，并把新的 `thread id` 写回 tmux metadata。
4. 保留当前 effective model、reasoning、plan mode。
5. 保留当前 pending model、reasoning、plan mode，让它们继续等待下一条真实用户输入生效。
6. 清空当前 pending approval 和最近 token usage snapshot。
7. `/status` 之后应直接看到新的 `thread id`；窗口统计在收到新 thread 的 token usage 事件前可以为空。

### 5.6 `/status`

返回至少包括：

- 当前 active session
- 当前 effective model / reasoning / plan mode
- 当前 pending model / reasoning / plan mode（仅在与 effective 不同时显示）
- 是否检测到 tmux 存活
- 是否存在本地 attach 占用
- 当前 cwd 与 git branch
- 当前 Codex CLI version
- 当前 thread id
- 当前 Context left / Window / Used tokens（基于 app-server 最近一次 token usage 通知）
- 若状态异常，下一步建议动作

### 5.6 `/projects`

返回至少包括：

- 允许打开的 project 名称
- 每个 project 对应的绝对路径

### 5.7 `/project_add` 与 `/project_remove`

约束：

- 命令名称必须使用 Telegram 兼容的下划线格式，而不是中划线。
- `/project_add` 只接受绝对路径。
- `/project_remove` 只允许删除动态添加的项目；环境变量中的静态项目不允许在聊天窗口中移除。

### 5.8 Mini App 目录浏览器

约束：

- Mini App 的目录选择器必须浏览 bridge 主机上的目录，而不是 Telegram 客户端设备上的目录。
- 目录选择器默认从当前 bridge 用户的 home 目录打开。
- 目录浏览支持返回父目录，直到 `/`。
- 快捷入口默认包含 `workspace`、`Personal`、`Playground`、`Home`、`Root`，并允许通过 `project_browse_roots` 追加。
- 浏览结果只返回子目录名称和绝对路径，不返回文件。
- 最终确认添加项目时，仍走 `/project_add <name> <abs-path>` 的既有语义与持久化链路。

## 6. 标准操作手册

### 6.1 创建 session

操作目标：为某个 project 建立第一个可交互 session。

执行语义：

1. 配置中存在该 project。
2. 收到 `/open <project>`。
3. 若 `codex-{project}` 不存在，则创建。
4. 在对应目录启动 Codex。
5. 将 active session 绑定到该 project。

成功判据：

- `/status` 可见当前 active session 已进入 `running` 或至少已脱离 `starting`。
- 后续 plain text 可以进入 Codex。

### 6.2 重连 tmux

适用场景：bridge 重启、app-server 连接中断、Telegram 侧短时失联。

执行语义：

1. 启动时枚举现有 `tmux session`。
2. 识别符合前缀的 `codex-{project}`。
3. 重建 session registry。
4. 通过 tmux session option 读取 app-server port 与 thread id。
5. 在用户重新 `/open <project>` 时优先 `thread/resume`，不主动重启 Codex。

成功判据：

- 原有 session 可在 `/list` 中重新出现。
- 用户重新 `/open <project>` 时优先绑定已有 session，而不是新建。

### 6.3 检测 Codex 存活

适用场景：用户怀疑会话已挂起，或 `/status` 需要给出恢复建议。

执行语义：

- 先检测 tmux session 是否存在。
- 再检测 session 内是否仍有活跃 Codex 交互迹象。
- 根据结果更新为 `running`、`exited` 或 `lost`。

输出要求：

- 告知用户是 bridge 丢附着、Codex 退出，还是 tmux 丢失。
- 给出下一步动作，例如重新 `/open <project>`。

### 6.4 Bridge 重启后 reattach

适用场景：bridge 进程重启或发布新版本。

执行语义：

1. 启动 bridge。
2. 从 `tmux` 恢复 session 列表。
3. 将没有 IM 绑定的已运行 session 记为 `detached` 或等价可恢复状态。
4. 等待用户通过 `/open <project>` 重新绑定 active session。

原则：

- reattach 是接管现有 session，不是重建新会话。
- Bridge 重启不能默认影响正在跑的 Codex。

### 6.5 Cloudflared tunnel 启动与重拉

适用场景：Telegram webhook 需要把公网请求转发回本机 `:8080`。

当前建议：

- 长期运行优先使用 named tunnel + 固定子域名。
- `quick tunnel` 只用于短期开发和临时联调。
- 不要求本机做公网端口映射。

推荐目标：

- 使用固定子域名，例如 `imtty.example.com`
- webhook 路径：`https://imtty.example.com/telegram/webhook`
- Mini App 路径：`https://imtty.example.com/mini-app`

named tunnel 标准启动步骤：

1. 启动 bridge：

```bash
GOCACHE=/tmp/imtty-go-build go run ./cmd/imtty-bridge
```

2. 登录 Cloudflare：

```bash
cloudflared tunnel login
```

3. 创建 named tunnel：

```bash
cloudflared tunnel create imtty
cloudflared tunnel list
```

4. 绑定 DNS：

```bash
cloudflared tunnel route dns imtty imtty.example.com
```

5. 写入 `~/.cloudflared/config.yml`：

```yaml
tunnel: imtty
credentials-file: /Users/<you>/.cloudflared/<TUNNEL-UUID>.json

ingress:
  - hostname: imtty.example.com
    service: http://127.0.0.1:8080
  - service: http_status:404
```

6. 确认 `config.toml` 中的公开基地址：

```toml
mini_app_base_url = "https://imtty.example.com"
```

7. 在另一个终端启动 tunnel：

```bash
cloudflared tunnel run imtty
```

8. 如需常驻运行，可安装为 macOS service：

```bash
cloudflared service install
```

系统级开机启动：

```bash
sudo cloudflared service install
```

9. 调 Telegram `setWebhook`：

```bash
curl -X POST "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/setWebhook" \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://imtty.example.com/telegram/webhook",
    "secret_token": "'"${IMTTY_TELEGRAM_WEBHOOK_SECRET}"'"
  }'
```

10. 验证 webhook 与本地健康状态：

```bash
curl -s "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/getWebhookInfo"
curl -s http://127.0.0.1:8080/healthz
```

期望：

- `getWebhookInfo` 返回的 URL 指向 `https://imtty.example.com/telegram/webhook`
- `healthz` 返回 `ok`
- bot `Menu Button` 指向 `https://imtty.example.com/mini-app`

quick tunnel 回退步骤：

1. 启动 bridge。
2. 在另一个终端启动 quick tunnel：

```bash
cloudflared tunnel --url http://127.0.0.1:8080
```

3. 从 `cloudflared` 输出中记录 `https://<random>.trycloudflare.com`。

4. 调 Telegram `setWebhook`：

```bash
curl -X POST "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/setWebhook" \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://<random>.trycloudflare.com/telegram/webhook",
    "secret_token": "'"${IMTTY_TELEGRAM_WEBHOOK_SECRET}"'"
  }'
```

5. 验证 webhook 状态：

```bash
curl -s "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/getWebhookInfo"
curl -s http://127.0.0.1:8080/healthz
```

恢复语义：

- 若只重启 bridge，而 tunnel 域名未变，则不需要重新 `setWebhook`。
- 若 named tunnel 保持同一 hostname，则 bridge 重启后 webhook 与 Mini App URL 不需要重写。
- 若 quick tunnel 重启后域名变化，则必须重新 `setWebhook`，并同步刷新 Mini App 基础地址。
- 若 `getWebhookInfo` 仍指向旧域名，Telegram 请求会继续发往旧 tunnel，当前 bridge 不会收到消息。

长期建议：

- quick tunnel 只适合开发和临时联调。
- 长期运行应切到 named tunnel + 固定域名。
- 建议使用子域名，不建议直接使用根域。
- 若启用了 Mini App，还应保证 `mini_app_base_url` 与 tunnel 的公开域名保持一致。

## 7. 异常与恢复流程

### 7.1 打开未知项目

- 提示该项目未在允许列表中。
- 返回可选 project 名称或提示用户检查配置。

### 7.2 无 active session 收到文本

- 拒绝透传。
- 明确提示先执行 `/open <project>`。

### 7.3 Codex 退出

- 将 session 状态设为 `exited`。
- `/status` 需要提示：“Codex 已退出，可重新 `/open <project>` 进入该项目会话。”
- `/restart` 只作为后续增强占位，不作为 MVP 对外命令。

### 7.4 tmux 丢失

- 将状态设为 `lost`。
- 提示用户当前后台 session 已不存在，需要重新 `/open <project>` 创建。

### 7.5 Telegram 发送受限或失败

- 输出队列不能因此阻塞全部 session。
- 记录失败并在下一次状态查询时保留必要提示。

## 8. 恢复原则

- 优先恢复已有 session，其次才是新建 session。
- 优先暴露真实状态，其次才是提供便捷包装。
- 所有恢复动作都要给出清晰、可执行的下一步提示。
- MVP 不引入自动恢复编排；恢复由用户触发，bridge 负责提供准确状态与附着能力。
