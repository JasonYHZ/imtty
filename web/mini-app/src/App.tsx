import { useEffect, useMemo, useState } from "react"
import { Terminal } from "lucide-react"
import { toast } from "sonner"

import {
  fetchBootstrap,
  fetchBrowse,
  postAction,
  type BootstrapResponse,
  type BrowseResponse,
  type ModelView,
  type ProjectView,
  type SessionView,
  type StatusView,
} from "@/lib/api"
import { getTelegramWebApp } from "@/lib/telegram"
import type { ModelOption, Project, Session, SessionStatus } from "@/lib/imtty-types"
import { Toaster } from "@/components/ui/sonner"
import { CurrentSession } from "@/components/imtty/current-session"
import { SessionSettings } from "@/components/imtty/session-settings"
import { SessionList } from "@/components/imtty/session-list"
import { ProjectList } from "@/components/imtty/project-list"
import { AddProject } from "@/components/imtty/add-project"

export function App() {
  const telegram = getTelegramWebApp()
  const [bootstrap, setBootstrap] = useState<BootstrapResponse | null>(null)
  const [browse, setBrowse] = useState<BrowseResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [browseLoading, setBrowseLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    telegram?.ready()
    telegram?.expand()
    void reload()
  }, [])

  useEffect(() => {
    if (bootstrap?.browse_default_path && !browse) {
      void loadBrowse(bootstrap.browse_default_path)
    }
  }, [bootstrap, browse])

  const projects = useMemo(
    () => (bootstrap?.projects ?? []).map(toProject),
    [bootstrap?.projects],
  )
  const sessions = useMemo(
    () =>
      (bootstrap?.sessions ?? []).map((session) =>
        toSession(session, bootstrap?.active_status),
      ),
    [bootstrap?.sessions, bootstrap?.active_status],
  )
  const currentSession = bootstrap?.active_session
    ? toSession(bootstrap.active_session, bootstrap.active_status)
    : null
  const modelOptions = useMemo(
    () => toModelOptions(bootstrap?.models ?? []),
    [bootstrap?.models],
  )

  async function reload() {
    if (!telegram?.initData) {
      setError("请从 Telegram 内部打开这个控制面板。")
      setLoading(false)
      return
    }

    setLoading(true)
    setError("")
    try {
      setBootstrap(await fetchBootstrap(telegram.initData))
    } catch (err) {
      const message = err instanceof Error ? err.message : "加载控制面板失败"
      setError(message)
      toast.error("加载失败", { description: message })
    } finally {
      setLoading(false)
    }
  }

  async function loadBrowse(path?: string) {
    if (!telegram?.initData) {
      setError("缺少 Telegram Mini App 上下文。")
      return
    }

    setBrowseLoading(true)
    setError("")
    try {
      setBrowse(await fetchBrowse(telegram.initData, path))
    } catch (err) {
      const message = err instanceof Error ? err.message : "加载目录浏览器失败"
      setError(message)
      toast.error("目录加载失败", { description: message })
    } finally {
      setBrowseLoading(false)
    }
  }

  async function runAction(endpoint: string, payload?: Record<string, string>) {
    if (!telegram?.initData) {
      setError("缺少 Telegram Mini App 上下文。")
      return
    }

    setSubmitting(true)
    setError("")
    try {
      const response = await postAction(telegram.initData, endpoint, payload)
      const text = response.responses.filter(Boolean).join("\n")
      if (text) {
        toast.success("操作完成", { description: text })
      }
      await reload()
    } catch (err) {
      const message = err instanceof Error ? err.message : "操作失败"
      setError(message)
      toast.error("操作失败", { description: message })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="mx-auto flex min-h-svh w-full max-w-md flex-col bg-background">
      <header className="sticky top-0 z-10 flex items-center justify-between border-b bg-background/90 px-4 py-3 backdrop-blur supports-[backdrop-filter]:bg-background/70">
        <div className="flex items-center gap-2">
          <span className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <Terminal className="h-4 w-4" aria-hidden="true" />
          </span>
          <div className="flex flex-col leading-tight">
            <span className="text-sm font-semibold tracking-tight">imtty</span>
            <span className="text-[10px] uppercase tracking-wider text-muted-foreground">
              Mini App
            </span>
          </div>
        </div>
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="h-1.5 w-1.5 rounded-full bg-blue-500" aria-hidden="true" />
          {loading ? "正在连接" : "已连接本机"}
        </div>
      </header>

      <main className="flex flex-1 flex-col gap-4 px-4 py-4">
        {error ? (
          <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
            {error}
          </div>
        ) : null}

        <CurrentSession
          session={currentSession}
          onClose={() => void runAction("/mini-app/api/close")}
          onDelete={() => void runAction("/mini-app/api/kill")}
          onClearContext={() => void runAction("/mini-app/api/clear")}
        />

        <SessionSettings
          session={currentSession}
          modelOptions={modelOptions}
          onChangeModel={(model) => void runAction("/mini-app/api/model", { model })}
          onChangeReasoning={(reasoning) =>
            void runAction("/mini-app/api/reasoning", { reasoning })
          }
          onChangePlanMode={(mode) =>
            void runAction("/mini-app/api/plan-mode", { mode })
          }
        />

        <SessionList
          sessions={sessions}
          currentSessionId={currentSession?.id ?? null}
          onSwitch={(sessionId) => {
            const target = sessions.find((session) => session.id === sessionId)
            if (target) {
              void runAction("/mini-app/api/open", { project: target.projectId })
            }
          }}
        />

        <ProjectList
          projects={projects}
          onOpen={(projectId) => void runAction("/mini-app/api/open", { project: projectId })}
          onDelete={(projectId) =>
            void runAction("/mini-app/api/project-remove", { name: projectId })
          }
        />

        <AddProject
          browse={browse}
          browseLoading={browseLoading || submitting}
          shortcuts={bootstrap?.browse_shortcuts ?? []}
          onJump={(path) => void loadBrowse(path)}
          onGoUp={() => {
            if (browse?.parent_absolute_path) {
              void loadBrowse(browse.parent_absolute_path)
            }
          }}
          onEnterDirectory={(directory) => void loadBrowse(directory.absolute_path)}
          onAdd={({ name, path }) =>
            runAction("/mini-app/api/project-add", { name, root: path })
          }
        />

        <p className="px-1 pb-2 text-center text-[11px] leading-relaxed text-muted-foreground">
          请在 Telegram 内打开本面板。回到聊天窗口即可继续与 Codex 对话。
        </p>
      </main>

      <Toaster position="top-center" richColors closeButton={false} />
    </div>
  )
}

function toProject(project: ProjectView): Project {
  return {
    id: project.name,
    name: project.name,
    path: project.root,
    kind: project.dynamic ? "dynamic" : "fixed",
  }
}

function toSession(session: SessionView, status?: StatusView): Session {
  const effective = status?.effective
  const usage = status?.token_usage
  const tokenLimit = usage?.context_window && usage.context_window > 0 ? usage.context_window : 1
  return {
    id: session.project,
    name: session.name,
    projectId: session.project,
    projectName: session.project,
    projectPath: session.root,
    status: toSessionStatus(session.state),
    threadId: status?.thread_id || "未读取",
    branch: status?.branch || "-",
    model: effective?.model || "未读取",
    reasoning: effective?.reasoning || "未读取",
    planMode: effective?.plan_mode === "plan" ? "plan" : "default",
    tokenUsage: usage?.total_tokens ?? 0,
    tokenLimit,
  }
}

function toSessionStatus(state: string): SessionStatus {
  switch (state) {
    case "running":
      return "running"
    case "starting":
      return "starting"
    case "exited":
      return "exited"
    case "idle":
      return "idle"
    default:
      return "disconnected"
  }
}

function toModelOptions(models: ModelView[]): ModelOption[] {
  if (models.length === 0) {
    return []
  }
  return models.map((model) => ({
    id: model.model || model.id,
    label: model.model || model.id,
    hint: `默认 reasoning: ${model.default_reasoning || "未读取"}`,
    supportedReasoning: model.supported ?? [],
  }))
}
