export interface Viewer {
  id: number
  username?: string
}

export interface SessionView {
  name: string
  project: string
  root: string
  state: string
}

export interface ProjectView {
  name: string
  root: string
  dynamic: boolean
}

export interface BrowseShortcutView {
  name: string
  path: string
}

export interface DirectoryEntryView {
  name: string
  absolute_path: string
}

export interface BrowseResponse {
  current_absolute_path: string
  parent_absolute_path?: string
  directories: DirectoryEntryView[]
  shortcuts: BrowseShortcutView[]
}

export interface StatusView {
  thread_id?: string
  cwd?: string
  branch?: string
  codex_version?: string
  effective: {
    model?: string
    reasoning?: string
    plan_mode?: string
  }
  pending: {
    model?: string
    reasoning?: string
    plan_mode?: string
  }
  target: {
    model?: string
    reasoning?: string
    plan_mode?: string
  }
  has_pending_controls: boolean
  token_usage: {
    context_window: number
    total_tokens: number
  }
  has_token_usage: boolean
  local_writable_attach: boolean
}

export interface ModelView {
  id: string
  model: string
  default_reasoning: string
  supported: string[]
}

export interface BootstrapResponse {
  viewer: Viewer
  active_session?: SessionView
  active_status?: StatusView
  models?: ModelView[]
  sessions: SessionView[]
  projects: ProjectView[]
  browse_default_path: string
  browse_shortcuts: BrowseShortcutView[]
}

export interface ActionResponse {
  ok: boolean
  responses: string[]
}

export async function fetchBootstrap(initData: string) {
  return request<BootstrapResponse>("/mini-app/api/bootstrap", {
    method: "GET",
    initData,
  })
}

export async function postAction(initData: string, endpoint: string, payload?: Record<string, string>) {
  return request<ActionResponse>(endpoint, {
    method: "POST",
    initData,
    body: payload ? JSON.stringify(payload) : undefined,
  })
}

export async function fetchBrowse(initData: string, path = "") {
  const query = new URLSearchParams()
  if (path) {
    query.set("path", path)
  }
  const suffix = query.size ? `?${query.toString()}` : ""
  return request<BrowseResponse>(`/mini-app/api/project-browse${suffix}`, {
    method: "GET",
    initData,
  })
}

async function request<T>(
  input: string,
  options: { method: string; initData: string; body?: string },
) {
  const response = await fetch(input, {
    method: options.method,
    headers: {
      "Content-Type": "application/json",
      "X-Telegram-Init-Data": options.initData,
    },
    body: options.body,
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(text || `request failed with status ${response.status}`)
  }

  return (await response.json()) as T
}
