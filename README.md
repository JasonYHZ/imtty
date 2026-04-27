# imtty

English | [简体中文](README.zh-CN.md)

`imtty` is a personal Telegram bridge for controlling local Codex sessions from any Telegram client.

It keeps Codex running on your own machine, inside `tmux`, and sends prompts, approvals, media, and final answers through Telegram. It is built for a single owner who wants remote access to the normal Codex workflow without exposing a raw shell or turning the project into an autonomous agent platform.

## Features

- Telegram webhook bridge for a single owner
- `tmux`-backed Codex sessions named `codex-{project}`
- Codex `app-server` integration with structured event output
- final-answer-first Telegram replies with terminal noise suppressed
- explicit human approval flow with `Yes` / `No` quick replies
- project whitelist with dynamic add/remove commands
- session reattach after bridge restart
- local desktop attach protection for writable `tmux` clients
- Telegram Mini App companion UI for session and project control
- image input from Telegram `photo` and image `document`
- text/code/PDF document analysis through temporary local files
- optional Telegram `voice` transcription through local `ffmpeg` and `whisper.cpp`

## Non-Goals

`imtty` is intentionally not:

- a hosted service
- a multi-user system
- an agent orchestrator
- a generic remote shell
- a web admin dashboard
- a file sync or long-term media storage service

## Architecture

```text
Telegram -> imtty bridge -> tmux session -> Codex app-server
                              |
                              +-> Telegram Mini App
```

Core rules:

- Telegram is the primary IM entrypoint.
- `tmux` is the only session backend.
- Codex must run inside the `tmux` session.
- Telegram output comes from Codex structured events, not terminal screen scraping.
- One Telegram chat can bind to one active session at a time.
- Multiple project sessions can exist at the same time.
- Codex approvals are never auto-confirmed by the bridge.

## Requirements

Verified local baseline:

- macOS
- Go `1.26.1`
- `tmux 3.6a`
- `codex-cli 0.125.0`
- Telegram bot token
- public HTTPS webhook URL, for example through Cloudflare Tunnel

Optional voice input:

- `ffmpeg`
- `whisper.cpp` `whisper-cli`
- local GGML whisper model

## Quick Start

### 1. Create config

```bash
cp config.toml.example config.toml
```

Edit at least:

```toml
telegram_bot_token = "BOT_TOKEN"
telegram_webhook_secret = "SECRET"
telegram_owner_id = 123456789
mini_app_base_url = "https://imtty.example.com"

[projects]
demo = "/absolute/path/to/your/project"
```

`config.toml` is ignored by git. Commit `config.toml.example`, not your local config.

### 2. Run the bridge

```bash
go run ./cmd/imtty-bridge
```

For a sandbox-friendly local build cache:

```bash
GOCACHE=/tmp/imtty-go-build go run ./cmd/imtty-bridge
```

Health check:

```bash
curl -s http://127.0.0.1:8080/healthz
```

### 3. Expose the webhook

A named Cloudflare Tunnel is the recommended long-running setup:

```bash
cloudflared tunnel login
cloudflared tunnel create imtty
cloudflared tunnel route dns imtty imtty.example.com
cloudflared tunnel run imtty
```

Example `~/.cloudflared/config.yml`:

```yaml
tunnel: imtty
credentials-file: /Users/<you>/.cloudflared/<tunnel-id>.json

ingress:
  - hostname: imtty.example.com
    service: http://127.0.0.1:8080
  - service: http_status:404
```

Set the Telegram webhook:

```bash
curl -X POST "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/setWebhook" \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://imtty.example.com/telegram/webhook",
    "secret_token": "'"${IMTTY_TELEGRAM_WEBHOOK_SECRET}"'"
  }'
```

Verify:

```bash
curl -s "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/getWebhookInfo"
curl -s http://127.0.0.1:8080/healthz
```

### 4. Talk to the bot

```text
/projects
/open demo
hello
```

## Voice Input

Voice input is disabled by default. Enable it only after configuring local transcription:

```toml
[voice]
enabled = true
ffmpeg_bin = "ffmpeg"
whisper_bin = "/opt/whisper.cpp/build/bin/whisper-cli"
model_path = "/opt/whisper.cpp/models/ggml-large-v3-turbo.bin"
language = "zh"
```

Behavior:

- Telegram `voice` files are downloaded to a local temp directory.
- `ffmpeg` converts the audio to 16 kHz mono WAV.
- `whisper.cpp` transcribes the audio locally.
- The transcript is submitted to the current active Codex session as plain text.
- The bridge does not keep long-term voice files.

Voice input does not implement voice commands, voice approvals, speaker diarization, long-audio jobs, or multi-message merging.

## Bot Commands

```text
/list
/projects
/project_add <name> <abs-path>
/project_remove <name>
/open <project> [thread-id]
/close
/kill
/clear
/status
/model [model-id]
/reasoning [effort]
/plan_mode [default|plan]
```

## Mini App

The Mini App is a lightweight companion UI inside Telegram. It supports:

- current active session view
- session list
- project list
- open / close / kill controls
- project add / remove
- host-side directory browser for selecting project roots

It does not replace the Telegram chat, live terminal output, prompt entry, or Codex approval flow.

The frontend lives in `web/mini-app/`. Built assets are checked in under `web/mini-app/dist/` so the Go bridge can serve them directly.

## Development

Run all Go tests:

```bash
GOPATH=/tmp/imtty-go GOMODCACHE=/tmp/imtty-go/pkg/mod GOCACHE=/tmp/imtty-go-build go test ./...
```

Build the Mini App:

```bash
cd web/mini-app
npm install
npm run build
```

## Security Notes

- Only whitelisted projects can be opened.
- Telegram webhook requests must include the configured secret token.
- Mini App requests must pass Telegram `initData` validation and owner checks.
- Images, documents, and voice files are stored only as temporary local files.
- The bridge refuses Telegram writes and `/kill` when the same `tmux` session has a writable local desktop attach.
- The bridge never auto-approves Codex permission prompts.

## Documentation

- [Product Requirements](docs/prd.md)
- [MVP Architecture Runbook](docs/mvp-architecture-runbook.md)
- [Guardrails](AGENTS.md)

## License

[MIT](LICENSE)
