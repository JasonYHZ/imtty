# imtty

English | [简体中文](README.zh-CN.md)

`imtty` is a Telegram-first bridge for running and controlling local Codex sessions from your phone or any Telegram client.

It is intentionally narrow in scope:

- single user
- local machine deployment
- Telegram as the primary remote interface
- `tmux` as the session carrier
- Codex running inside `tmux`
- human-in-the-loop approval flow preserved

The project is not trying to become a multi-user agent platform, a hosted control plane, or a generic file-sync product.

## Status

This repository already contains a working baseline, not just design docs.

Implemented today:

- Telegram webhook bridge
- `tmux` session lifecycle management
- Codex `app-server` integration
- final-only Telegram replies by default
- Telegram Mini App companion UI
- dynamic project whitelist management
- image input:
  - Telegram `photo`
  - image `document`
- file analysis input:
  - text and code documents
  - PDF text extraction
- native Telegram `typing` indicator while a turn is in progress
- session reattach after bridge restart
- local-attach protection:
  - writable desktop attach blocks Telegram writes
  - readonly desktop spectator attach does not block Telegram writes

## What Problem It Solves

When you step away from your desktop, you may still want to:

- send another prompt to Codex
- read the final answer
- approve a command or permission request
- reopen or switch between project sessions
- inspect and manage sessions from a lightweight UI

`imtty` gives you that without exposing a raw shell, and without introducing autonomous orchestration that changes the core Codex interaction model.

## Architecture

```text
Telegram -> imtty bridge -> tmux session -> Codex app-server
                              |
                              +-> Telegram Mini App
```

Key design choices:

- `tmux` is the only session backend
- Telegram default output comes from Codex structured events, not terminal screen scraping
- approvals remain explicit
- one Telegram chat has one active session at a time
- multiple project sessions may exist concurrently

## Features

### Sessions

- `/open <project>` creates or binds `codex-{project}`
- `/open <project> <thread-id>` strictly resumes the given Codex thread id and binds it as the project's current thread
- `/close` detaches the current active session
- `/kill` terminates the underlying session and removes it from the known session list
- `/close` returns the current thread id before detaching
- `/kill` returns the current thread id before deleting the session
- `/clear` starts a fresh thread for the current active session without recreating the tmux session
- `/status` shows the current active session details and current Codex window stats
- `/list` lists sessions only
- `/projects` lists allowed projects only
- `/model [model-id]` shows or updates the active session model override
- `/reasoning [effort]` shows or updates the active session reasoning override
- `/plan_mode [default|plan]` shows or updates the active session bridge-side plan preset

### Project Whitelist

- `/project_add <name> <abs-path>`
- `/project_remove <name>`
- dynamic projects are persisted locally in `.imtty-projects.json`
- static projects still come from config or environment

### Telegram Chat UX

- plain text goes to the current active session
- successful submission is silent by default
- Telegram shows native `typing` while Codex is working
- only final answers, approval prompts, and essential status are sent back
- terminal noise is suppressed by default
- model / reasoning / plan-mode changes are queued on the active session and applied with the next real turn
- `/clear` immediately switches the active session to a new empty thread while keeping the tmux session and effective controls
- writable local desktop attach blocks remote writes and remote kill
- readonly spectator mode is supported with `tmux attach -r -t codex-<project>`

### Image Input

Supported:

- Telegram `photo`
- image `document`

Behavior:

- downloaded to a local temp directory
- submitted to Codex as `localImage`
- optional caption is sent in the same turn

### File Analysis Input

Supported:

- text files
- code files
- PDF documents

Behavior:

- files are downloaded to a local temp directory
- text and code files are read and sent as text input
- PDF files are locally converted to text first, then sent as text input
- unsupported binary files are rejected

Current limits:

- no arbitrary binary attachment support
- no scanned-PDF OCR in the first version
- no long-term media storage

### Mini App

The Mini App is a companion control surface inside Telegram. It is not a second terminal.

Supported today:

- current active session
- session list
- project list
- open / close / kill actions
- project add / remove
- host-side directory browser for picking project roots

Not supported in the Mini App:

- live terminal output
- free-form prompt entry
- approval flow replacement

## Requirements

Verified local baseline:

- `tmux 3.6a`
- `codex-cli 0.125.0`
- `go 1.26.1`

Also expected:

- a Telegram bot token
- an HTTPS webhook endpoint reachable by Telegram
- `cloudflared` or an equivalent tunnel/domain setup

## Quick Start

### 1. Prepare config

Copy the example:

```bash
cp config.toml.example config.toml
```

Edit at least:

- `telegram_bot_token`
- `telegram_webhook_secret`
- `telegram_owner_id`
- `mini_app_base_url`
- `[projects]`

### 2. Run the bridge

```bash
GOCACHE=/tmp/imtty-go-build go run ./cmd/imtty-bridge
```

Health check:

```bash
curl -s http://127.0.0.1:8080/healthz
```

### 3. Expose it with Cloudflare Tunnel

Recommended for long-running use: a named tunnel with a fixed hostname such as `imtty.example.com`.

#### 3.1 Authenticate `cloudflared`

```bash
cloudflared tunnel login
```

This opens a browser, authenticates against your Cloudflare account, and installs the local certificate used to manage DNS-backed tunnels.

#### 3.2 Create a named tunnel

```bash
cloudflared tunnel create imtty
cloudflared tunnel list
```

Record the tunnel UUID and the generated credentials file path.

#### 3.3 Route a fixed hostname

Example:

```bash
cloudflared tunnel route dns imtty imtty.example.com
```

This creates a DNS record pointing your hostname at the tunnel.

#### 3.4 Create `~/.cloudflared/config.yml`

```yaml
tunnel: imtty
credentials-file: /Users/<you>/.cloudflared/<TUNNEL-UUID>.json

ingress:
  - hostname: imtty.example.com
    service: http://127.0.0.1:8080
  - service: http_status:404
```

#### 3.5 Point `imtty` at the fixed public URL

Set `mini_app_base_url` in `config.toml`:

```toml
mini_app_base_url = "https://imtty.example.com"
```

#### 3.6 Run the tunnel

Foreground:

```bash
cloudflared tunnel run imtty
```

Run at login on macOS:

```bash
cloudflared service install
```

Run at boot on macOS:

```bash
sudo cloudflared service install
```

#### 3.7 Set the Telegram webhook

```bash
curl -X POST "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/setWebhook" \
  -H 'Content-Type: application/json' \
  -d '{
    "url": "https://imtty.example.com/telegram/webhook",
    "secret_token": "'"${IMTTY_TELEGRAM_WEBHOOK_SECRET}"'"
  }'
```

#### 3.8 Verify public routing

```bash
curl -s "https://api.telegram.org/bot${IMTTY_TELEGRAM_BOT_TOKEN}/getWebhookInfo"
curl -s http://127.0.0.1:8080/healthz
```

Expected:

- webhook URL points to `https://imtty.example.com/telegram/webhook`
- local health check returns `ok`
- the bot menu button opens `https://imtty.example.com/mini-app`

#### 3.9 Quick tunnel for short-lived development

Use this only for temporary local testing:

```bash
cloudflared tunnel --url http://127.0.0.1:8080
```

If the `trycloudflare.com` hostname changes, you must update the Telegram webhook and the Mini App base URL.

### 4. Talk to the bot

Typical flow:

```text
/projects
/open my-project
hello
```

## Configuration

Default startup reads `config.toml` from the current working directory.

Override options:

- `-config /abs/path/to/config.toml`
- `-listen :9090`
- `IMTTY_*` environment variables

Important:

- `config.toml` is intentionally ignored by git
- only `config.toml.example` should be committed

## Deployment Notes

- Use a named Cloudflare Tunnel and a fixed hostname for normal operation.
- Prefer a subdomain such as `imtty.example.com`, not the apex domain.
- `mini_app_base_url` should match the same public hostname used by the tunnel.
- Bridge and tunnel are separate processes. Restarting one does not automatically restart the other.
- If the public hostname changes, refresh:
  - Telegram webhook
  - Mini App menu button target

## Public Interfaces

### Bot Commands

- `/list`
- `/projects`
- `/project_add <name> <abs-path>`
- `/project_remove <name>`
- `/open <project>`
- `/open <project> <thread-id>`
- `/close`
- `/kill`
- `/clear`
- `/status`
- `/model [model-id]`
- `/reasoning [effort]`
- `/plan_mode [default|plan]`

### HTTP Endpoints

- `POST /telegram/webhook`
- `GET /healthz`
- `GET /mini-app`
- `GET/POST /mini-app/api/*`

## Security Model

This project is designed for a single owner running on their own machine.

Current safeguards:

- Telegram webhook secret validation
- Mini App `initData` verification
- owner ID check for Mini App access
- project whitelist instead of arbitrary `/open /any/path`
- temp-file storage for images and documents
- no automatic approval
- no remote writes when the same session is attached locally on desktop

## Development

Run tests:

```bash
GOPATH=/tmp/imtty-go GOMODCACHE=/tmp/imtty-go/pkg/mod GOCACHE=/tmp/imtty-go-build go test ./...
```

Mini App frontend lives under:

- `web/mini-app/`

The repository currently checks in `web/mini-app/dist/` so the Go bridge can serve the built assets directly. The Mini App is a static frontend build that calls bridge-owned `/mini-app/api/*` endpoints at runtime. After `npm run build`, refreshed Mini App requests read the updated dist files without restarting the bridge process.

## Documentation

- [Product Requirements](./docs/prd.md)
- [MVP Architecture Runbook](./docs/mvp-architecture-runbook.md)
- [Guardrails](./AGENTS.md)

## Non-Goals

`imtty` does not currently aim to provide:

- multi-user access control
- autonomous agent orchestration
- a generic web admin panel
- arbitrary file upload workflows
- database-backed state as a prerequisite
- replacement of Codex native approvals

## License

[MIT](./LICENSE)
