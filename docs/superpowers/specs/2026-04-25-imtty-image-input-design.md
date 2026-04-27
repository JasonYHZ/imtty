# imtty Image Input Design

## Summary

在当前 `Telegram-first + tmux + Codex app-server` 架构下，新增最小图片输入支持：

- 支持 Telegram `photo`
- 支持 MIME 为图片的 `document`
- 下载到 bridge 主机的临时目录
- 通过 Codex app-server 的 `localImage` 输入发给 Codex
- 如果消息带 caption，则与图片一起作为同一轮输入提交

本设计不引入长期媒体存储，不改变单用户边界，也不把图片复制进项目目录。

## Scope

第一版只做：

- 单张 `photo`
- 单张图片型 `document`
- JPEG / PNG / WebP
- 临时文件落地
- caption 与图片同轮输入

不做：

- 多图消息
- 视频 / GIF / sticker
- 语音输入另见 PRD 和 MVP runbook 的 voice 边界
- OCR
- 图片长期留档
- Mini App 图片上传

## Input model

### Telegram `photo`

- Telegram `photo` 会给出多尺寸数组。
- bridge 只取最大的一张。
- `caption` 作为可选文本输入。

### Telegram `document`

- 只接受图片 MIME：
  - `image/jpeg`
  - `image/png`
  - `image/webp`
- 非图片 `document` 直接拒绝，并返回“当前只支持图片文件”。
- 如果存在 `caption`，与图片一起提交。

## Bridge flow

1. webhook 解析 `message.photo` / `message.document`
2. 通过 Telegram `getFile` 获取 `file_path`
3. 下载文件到 bridge 主机临时目录
4. 做最小校验：
   - MIME 白名单
   - 文件大小上限
   - 扩展名推断
5. 生成 app-server 输入：
   - `localImage`
   - 可选 `text`
6. 通过 `turn/start` 提交给 Codex

## Temporary file strategy

- 临时目录默认放在 `os.TempDir()/imtty-media`
- 子目录按 session 划分，避免不同项目会话混在一起
- 文件名使用时间戳 + Telegram file id，避免碰撞
- 不复制到项目目录
- 不做长期持久化

清理策略：

- 每次写入新图片前，顺手清理 24 小时前的旧临时文件
- 不要求 turn 完成即删，避免 Codex 仍在读取时被删掉

## Runtime changes

### `internal/telegram`

- `Message` 新增 `caption`、`photo[]`、`document`
- `Adapter` 新增图片消息分支
- `Adapter` 依赖一个 Telegram file client 来：
  - `getFile`
  - 下载原始文件

### `internal/media`

新增临时文件存储组件，负责：

- 根据 session 写入临时图片
- 推断扩展名
- 顺手清理过期临时文件

### `internal/appserver`

- `Client` 新增多输入 turn 提交
- `Runtime` 新增 `SubmitImage(...)`
- 普通文本仍走原有 `SubmitText(...)`

## Error handling

- 无 active session：提示先 `/open <project>`
- Telegram `getFile` 失败：提示“获取图片失败，请重试”
- 下载失败：提示“下载图片失败，请重试”
- 非图片 `document`：提示“当前只支持图片文件”
- 临时文件写入失败：提示“保存临时图片失败”
- app-server 提交失败：提示“发送图片到会话失败”

## Acceptance criteria

1. Telegram `photo` 能作为单张图片输入进入当前 active session。
2. Telegram 图片型 `document` 能进入当前 active session。
3. 非图片 `document` 会被明确拒绝。
4. 图片带 caption 时，caption 会和图片一起进入同一轮 Codex 输入。
5. bridge 只写入临时目录，不改项目目录。
6. 过期临时图片会被顺手清理，不长期累积。
