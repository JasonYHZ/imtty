# imtty Mini App Design

日期：2026-04-25

## 1. 目标

为 `imtty` 增加一个 Telegram Mini App 轻量控制面板，服务单人、本机、Telegram-first 的使用场景。

第一版 Mini App 只承担结构化控制入口，不替代 Telegram 聊天窗口中的文本交互。

用户可以在 Mini App 中：

- 查看当前 active session
- 查看已知 session 列表
- 查看可打开项目列表
- 执行 `open / close / kill`
- 执行 `clear`
- 查看并切换当前会话的 model / reasoning / plan mode
- 执行 `project_add / project_remove`

用户仍然在 Telegram 聊天窗口中：

- 查看实时 Codex 输出
- 发送普通文本给 Codex
- 处理审批流与 `是/否` 快捷回复

## 2. 设计原则

- Telegram-first：Mini App 是 Telegram bot 的附属入口，不是独立产品。
- Single-user：默认只允许一个 owner 使用。
- Session-first：Mini App 操作的仍然是当前 bridge 内的 session 真相源。
- UI companion：Mini App 是结构化控制面板，不是第二个终端。
- Minimal surface：第一版只做单页，不做复杂导航。

## 3. 范围

### 3.1 第一版必须包含

- Telegram bot `Menu Button` 打开 Mini App
- 单页控制面板
- 当前 active session 卡片
- session 列表
- project 列表
- 添加项目表单
- 删除动态项目按钮
- `open / close / kill / project_add / project_remove` 控制动作
- `clear / model / reasoning / plan_mode` 控制动作
- Telegram Mini App `initData` 服务端校验
- owner 白名单控制

### 3.2 第一版明确不做

- 不做实时 stdout 流展示
- 不做普通文本输入框
- 不做审批流替代
- 不做多页面路由
- 不做数据库
- 不做多用户
- 不做独立后台管理系统
- 不做与现有 slash command 语义冲突的第二套控制逻辑

## 4. 用户体验

Mini App 首页采用单页布局，分 4 个区块。

### 4.1 当前会话

显示：

- active session 名称
- project 名称
- 当前状态

动作：

- `关闭当前会话`
- `清空当前会话上下文`
- `彻底删除当前会话`

### 4.2 会话列表

每项显示：

- session 名称
- project 名称
- 状态

动作：

- `打开并切换`
- 若当前项是 active，则只显示状态，不重复给出 `open`

### 4.2.1 会话设置

显示：

- 当前 model
- 当前 reasoning
- 当前 plan mode

动作：

- 切换 model
- 切换 reasoning
- 切换 plan mode

所有设置只对当前 active session 生效，并在下一条真实消息时应用。

### 4.3 项目列表

每项显示：

- project 名称
- 绝对路径
- 是否为动态项目

动作：

- `打开`
- 若是动态项目，则显示 `删除项目`

### 4.4 添加项目

表单字段：

- 项目名
- 目录选择器

动作：

- `添加项目`

目录选择器约束：

- 不使用 Telegram 客户端设备侧的文件或目录 picker。
- 由 bridge 提供系统级目录浏览能力。
- 默认从 `~` 打开。
- 顶部展示一组快捷入口：`workspace`、`Personal`、`Playground`、`Home`、`Root`。
- 用户可以在任意目录逐级浏览子目录，并一路返回父目录直到 `/`。
- 最终提交时仍写入项目名和选中的绝对路径。

## 5. 信息架构

Mini App 不做多页面路由。

页面打开后立即请求 bootstrap 数据，并渲染：

- `active_session`
- `sessions`
- `projects`

每次动作完成后，前端重新请求 bootstrap，保持状态简单一致。

## 6. 后端接口

新增一组只服务 Mini App 的轻量 API。

### 6.1 页面入口

- `GET /mini-app`

返回静态前端页面。

### 6.2 结构化接口

- `GET /mini-app/api/bootstrap`
- `POST /mini-app/api/open`
- `POST /mini-app/api/close`
- `POST /mini-app/api/kill`
- `POST /mini-app/api/clear`
- `POST /mini-app/api/model`
- `POST /mini-app/api/reasoning`
- `POST /mini-app/api/plan-mode`
- `POST /mini-app/api/project-add`
- `POST /mini-app/api/project-remove`
- `GET /mini-app/api/project-browse`

### 6.3 接口语义

`GET /mini-app/api/bootstrap` 返回：

- `viewer`
- `active_session`
- `sessions`
- `projects`
- `browse_default_path`
- `browse_shortcuts`
- `active_status`
- `models`

`POST /mini-app/api/open` 请求体：

```json
{ "project": "imtty" }
```

`POST /mini-app/api/project-add` 请求体：

```json
{ "name": "demo", "root": "/abs/path" }
```

`GET /mini-app/api/project-browse` 查询参数：

```text
path=/Users/jasonyu/workspace/demo
```

返回：

- 当前目录绝对路径
- 父目录绝对路径
- 子目录列表
- 快捷入口列表

`POST /mini-app/api/project-remove` 请求体：

```json
{ "name": "demo" }
```

`POST /mini-app/api/model` 请求体：

```json
{ "model": "gpt-5.5" }
```

`POST /mini-app/api/reasoning` 请求体：

```json
{ "reasoning": "high" }
```

`POST /mini-app/api/plan-mode` 请求体：

```json
{ "mode": "plan" }
```

接口动作必须复用现有 bridge 语义，不允许新造一套 session 生命周期。

## 7. 安全模型

### 7.1 入口限制

Mini App 只通过 Telegram bot 的 `Menu Button` 打开。

### 7.2 鉴权方式

前端从 Telegram WebApp 上下文读取 `initData`，每次 API 请求都带给服务端。

服务端按 Telegram Mini App 官方规则验证：

- `auth_date`
- `user`
- `hash`

只有校验通过后才允许访问 Mini App API。

### 7.3 Owner 白名单

新增配置：

- `IMTTY_TELEGRAM_OWNER_ID`

校验通过后，还必须要求当前 Telegram user id 等于 owner id。

### 7.4 安全边界

- Mini App API 不复用 webhook secret 作为鉴权方式。
- `IMTTY_TELEGRAM_WEBHOOK_SECRET` 只用于 Telegram webhook。
- Mini App URL 即使被别人拿到，没有合法 `initData` 与 owner 身份也不能操作 bridge。

## 8. 技术路线

### 8.1 前端

第一版前端技术路线固定为：

- React
- Vite
- Tailwind CSS
- shadcn/ui
- Radix UI

要求：

- 组件优先，不手写大段 CSS
- 只保留少量主题变量和必要布局样式
- 危险动作必须有确认交互

建议使用组件：

- `Card`
- `Button`
- `Badge`
- `Input`
- `Label`
- `Alert`
- `AlertDialog`
- `Separator`
- `ScrollArea`

### 8.2 后端

继续使用当前 Go bridge 单进程提供：

- webhook
- Mini App API
- Mini App 静态资源

不新增第二个部署单元。

## 9. 代码结构

建议目录结构：

- `cmd/imtty-bridge/`
- `internal/miniapp/`
- `internal/config/`
- `internal/session/`
- `internal/telegram/`
- `web/mini-app/`

其中：

- `internal/miniapp/` 负责 Mini App 的 HTTP handler、鉴权中间件、请求响应结构
- `web/mini-app/` 负责前端单页应用

## 10. 配置项

新增最小配置：

- `IMTTY_TELEGRAM_OWNER_ID`
- `IMTTY_MINI_APP_BASE_URL`

说明：

- `IMTTY_TELEGRAM_OWNER_ID` 用于限制唯一 owner
- `IMTTY_MINI_APP_BASE_URL` 用于生成 bot `Menu Button` 指向的 Mini App 地址
- `project_browse_roots` 用于声明 Mini App 额外快捷入口

## 11. 运维与发布

Mini App 与现有 webhook 共享同一个外网入口。

典型路径：

- `POST /telegram/webhook`
- `GET /mini-app`
- `GET /mini-app/api/bootstrap`
- `POST /mini-app/api/*`

如果继续使用 `cloudflared` quick tunnel：

- tunnel 域名变化后，不仅 webhook 需要更新
- Mini App 的 `Menu Button` URL 也需要同步改到新的域名

## 12. 验收标准

以下场景全部通过，Mini App 第一版才算完成：

1. 从 bot 的 `Menu Button` 可以打开 Mini App。
2. Mini App 能显示当前 active session、session 列表、project 列表。
3. 点击 `打开` 后，session 状态能刷新，并与聊天窗口 `/status` 语义一致。
4. 点击 `关闭当前会话` 与 `彻底删除当前会话` 后，状态能立即刷新。
5. 通过表单添加项目后，项目立即出现在列表中，bridge 重启后仍保留。
6. 通过目录选择器可以从 `~` 开始浏览，并一路返回到 `/` 后继续选择目录。
7. 目录浏览不会返回文件项。
8. 删除动态项目后，项目从列表消失，且不能再 `open`。
9. 环境变量静态项目不能在 Mini App 中被删除。
10. 非 owner 用户即使拿到 Mini App URL，也无法通过 Mini App API 控制 bridge。
11. Mini App 不影响聊天窗口中的 stdout、普通文本输入和审批流。

## 13. 实现顺序

1. 先在文档中正式打开 Mini App companion UI 边界。
2. 先实现 `internal/miniapp/` 鉴权与 API。
3. 再实现 `bootstrap / open / close / kill / project add / project remove`。
4. 然后搭建前端单页，先接结构化状态与按钮动作。
5. 最后配置 Telegram bot 的 `Menu Button`。

## 14. 备注

这版 Mini App 的目标不是“替代 Telegram 对话”，而是“减少 slash command 和记忆负担”。

真正的文本交互真相源仍然是 Telegram 聊天窗口与现有 bridge。
