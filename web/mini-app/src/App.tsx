import { FormEvent, type ReactNode, useEffect, useState } from "react"
import {
  fetchBootstrap,
  fetchBrowse,
  postAction,
  type BootstrapResponse,
  type BrowseResponse,
  type BrowseShortcutView,
  type DirectoryEntryView,
  type ProjectView,
  type SessionView,
} from "@/lib/api"
import { getTelegramWebApp } from "@/lib/telegram"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import {
  AlertDialog,
  AlertDialogActionButton,
  AlertDialogCancelButton,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"

export function App() {
  const telegram = getTelegramWebApp()
  const [bootstrap, setBootstrap] = useState<BootstrapResponse | null>(null)
  const [browse, setBrowse] = useState<BrowseResponse | null>(null)
  const [selectedProjectRoot, setSelectedProjectRoot] = useState("")
  const [loading, setLoading] = useState(true)
  const [browseLoading, setBrowseLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState("")
  const [notice, setNotice] = useState("")
  const [projectName, setProjectName] = useState("")

  useEffect(() => {
    telegram?.ready()
    telegram?.expand()
    void reload()
  }, [])

  useEffect(() => {
    if (!bootstrap?.browse_default_path) {
      return
    }
    if (!browse) {
      void loadBrowse(bootstrap.browse_default_path)
    }
  }, [bootstrap, browse])

  async function reload() {
    if (!telegram?.initData) {
      setError("请从 Telegram 内部打开这个控制面板。")
      setLoading(false)
      return
    }

    setLoading(true)
    setError("")
    try {
      const next = await fetchBootstrap(telegram.initData)
      setBootstrap(next)
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载控制面板失败")
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
      const next = await fetchBrowse(telegram.initData, path)
      setBrowse(next)
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载目录浏览器失败")
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
    setNotice("")
    try {
      const response = await postAction(telegram.initData, endpoint, payload)
      setNotice(response.responses.join("\n"))
      await reload()
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败")
    } finally {
      setSubmitting(false)
    }
  }

  async function handleAddProject(event: FormEvent) {
    event.preventDefault()
    await runAction("/mini-app/api/project-add", {
      name: projectName,
      root: selectedProjectRoot,
    })
    setProjectName("")
    setSelectedProjectRoot("")
  }

  function handleSelectCurrentDirectory() {
    if (!browse) {
      return
    }
    setSelectedProjectRoot(browse.current_absolute_path)
    if (!projectName) {
      setProjectName(lastPathSegment(browse.current_absolute_path))
    }
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-5xl flex-col gap-4 px-4 py-6 md:px-6">
      <header className="rounded-[28px] border border-white/60 bg-[linear-gradient(135deg,#fffaf3_0%,#f4ead8_70%,#ecd4b8_100%)] p-6 shadow-[0_22px_50px_-32px_rgba(82,49,21,0.4)]">
        <p className="text-sm font-medium uppercase tracking-[0.3em] text-[color:var(--primary)]">imtty Mini App</p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight">会话与项目控制面板</h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-[color:var(--muted-foreground)]">
          这里负责结构化控制。实时输出、普通文本输入和审批流仍然留在 Telegram 聊天窗口。
        </p>
      </header>

      {error ? <Banner tone="error" text={error} /> : null}
      {notice ? <Banner tone="info" text={notice} /> : null}

      <section className="grid gap-4 md:grid-cols-[1.1fr_1fr]">
        <Card>
          <CardHeader>
            <CardTitle>当前会话</CardTitle>
            <CardDescription>当前 active session 的绑定与危险动作。</CardDescription>
          </CardHeader>
          <CardContent>
            {loading ? (
              <p className="text-sm text-[color:var(--muted-foreground)]">正在加载当前状态…</p>
            ) : bootstrap?.active_session ? (
              <div className="space-y-4">
                <SessionMeta session={bootstrap.active_session} />
                <div className="flex flex-wrap gap-2">
                  <Button variant="secondary" disabled={submitting} onClick={() => void runAction("/mini-app/api/close")}>
                    关闭当前会话
                  </Button>
                  <ConfirmAction
                    title="彻底删除当前会话？"
                    description="这会终止底层 tmux/Codex，并删除当前 session 记录。"
                    triggerLabel="彻底删除当前会话"
                    variant="destructive"
                    disabled={submitting}
                    onConfirm={() => void runAction("/mini-app/api/kill")}
                  />
                </div>
              </div>
            ) : (
              <p className="text-sm text-[color:var(--muted-foreground)]">当前没有 active session。</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>添加项目</CardTitle>
            <CardDescription>像目录选择器一样浏览 bridge 主机目录，再把当前目录加入动态白名单。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={(event) => void handleAddProject(event)}>
              <label className="block space-y-1">
                <span className="text-sm font-medium">项目名</span>
                <Input value={projectName} onChange={(event) => setProjectName(event.target.value)} placeholder="demo" />
              </label>

              <DirectoryPicker
                browse={browse}
                browseLoading={browseLoading}
                shortcuts={bootstrap?.browse_shortcuts ?? []}
                selectedProjectRoot={selectedProjectRoot}
                onJump={(path) => {
                  setSelectedProjectRoot("")
                  void loadBrowse(path)
                }}
                onGoUp={() => {
                  if (!browse?.parent_absolute_path) {
                    return
                  }
                  setSelectedProjectRoot("")
                  void loadBrowse(browse.parent_absolute_path)
                }}
                onEnterDirectory={(directory) => {
                  setSelectedProjectRoot("")
                  void loadBrowse(directory.absolute_path)
                }}
                onSelectCurrent={handleSelectCurrentDirectory}
                onClearSelection={() => setSelectedProjectRoot("")}
              />

              <Button className="w-full" disabled={submitting || !projectName || !selectedProjectRoot} type="submit">
                添加项目
              </Button>
            </form>
          </CardContent>
        </Card>
      </section>

      <section className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>会话列表</CardTitle>
            <CardDescription>已知 session 与状态，支持重新打开并切换。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {loading ? (
              <p className="text-sm text-[color:var(--muted-foreground)]">正在加载会话列表…</p>
            ) : bootstrap?.sessions.length ? (
              bootstrap.sessions.map((item) => (
                <RowCard
                  key={item.name}
                  title={item.name}
                  subtitle={item.root}
                  badge={badgeForState(item.state)}
                  action={
                    bootstrap.active_session?.project === item.project ? (
                      <Button disabled size="sm" variant="secondary">
                        当前会话
                      </Button>
                    ) : (
                      <Button size="sm" disabled={submitting} onClick={() => void runAction("/mini-app/api/open", { project: item.project })}>
                        打开并切换
                      </Button>
                    )
                  }
                />
              ))
            ) : (
              <p className="text-sm text-[color:var(--muted-foreground)]">会话列表为空。</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>项目列表</CardTitle>
            <CardDescription>允许打开的项目，以及动态项目的删除入口。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {loading ? (
              <p className="text-sm text-[color:var(--muted-foreground)]">正在加载项目列表…</p>
            ) : (
              bootstrap?.projects.map((project) => (
                <ProjectRow
                  key={project.name}
                  project={project}
                  submitting={submitting}
                  onOpen={() => void runAction("/mini-app/api/open", { project: project.name })}
                  onRemove={() => void runAction("/mini-app/api/project-remove", { name: project.name })}
                />
              ))
            )}
          </CardContent>
        </Card>
      </section>
    </main>
  )
}

function DirectoryPicker({
  browse,
  browseLoading,
  shortcuts,
  selectedProjectRoot,
  onJump,
  onGoUp,
  onEnterDirectory,
  onSelectCurrent,
  onClearSelection,
}: {
  browse: BrowseResponse | null
  browseLoading: boolean
  shortcuts: BrowseShortcutView[]
  selectedProjectRoot: string
  onJump: (path: string) => void
  onGoUp: () => void
  onEnterDirectory: (directory: DirectoryEntryView) => void
  onSelectCurrent: () => void
  onClearSelection: () => void
}) {
  return (
    <div className="space-y-3">
      <div className="space-y-2">
        <p className="text-sm font-medium">快捷区域</p>
        <div className="flex flex-wrap gap-2">
          {shortcuts.map((shortcut) => (
            <Button key={`${shortcut.name}:${shortcut.path}`} size="sm" type="button" variant="outline" onClick={() => onJump(shortcut.path)}>
              {shortcut.name}
            </Button>
          ))}
        </div>
      </div>

      <div className="rounded-2xl border border-[color:var(--border)] bg-white/55">
        <div className="border-b border-[color:var(--border)] px-4 py-3">
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" type="button" variant="outline" disabled={!browse?.parent_absolute_path || browseLoading} onClick={onGoUp}>
              上一级
            </Button>
            <Button size="sm" type="button" variant="secondary" disabled={!browse || browseLoading} onClick={onSelectCurrent}>
              选择当前目录
            </Button>
            {selectedProjectRoot ? (
              <Button size="sm" type="button" variant="outline" onClick={onClearSelection}>
                清空已选
              </Button>
            ) : null}
          </div>
          <div className="mt-3 rounded-xl border border-[color:var(--border)] bg-[color:var(--muted)] px-3 py-2">
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-[color:var(--primary)]">当前路径</p>
            <p className="mt-1 break-all font-mono text-sm text-[color:var(--foreground)]">
              {browse?.current_absolute_path ?? "正在准备目录浏览器…"}
            </p>
          </div>
        </div>

        <div className="h-72 overflow-y-auto">
          {browseLoading ? (
            <div className="px-4 py-4 text-sm text-[color:var(--muted-foreground)]">正在加载目录…</div>
          ) : !browse ? (
            <div className="px-4 py-4 text-sm text-[color:var(--muted-foreground)]">正在初始化目录选择器…</div>
          ) : browse.directories.length ? (
            browse.directories.map((directory) => (
              <button
                key={directory.absolute_path}
                className="flex w-full items-center justify-between border-b border-[color:var(--border)] px-4 py-3 text-left transition-colors hover:bg-[color:var(--muted)]"
                onClick={() => onEnterDirectory(directory)}
                type="button"
              >
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">{directory.name}</span>
                  <span className="mt-1 block truncate text-xs text-[color:var(--muted-foreground)]">{directory.absolute_path}</span>
                </span>
                <span className="ml-4 text-sm text-[color:var(--muted-foreground)]">进入</span>
              </button>
            ))
          ) : (
            <div className="px-4 py-4 text-sm text-[color:var(--muted-foreground)]">当前目录没有子目录。</div>
          )}
        </div>
      </div>

      <div className="rounded-2xl border border-[#d7c8ae] bg-[#fff9ef] px-4 py-3">
        <p className="text-xs font-medium uppercase tracking-[0.18em] text-[color:var(--primary)]">已选目录</p>
        <p className="mt-1 break-all text-sm text-[color:var(--foreground)]">{selectedProjectRoot || "尚未选择目录"}</p>
      </div>
    </div>
  )
}

function SessionMeta({ session }: { session: SessionView }) {
  return (
    <div className="rounded-2xl border border-[color:var(--border)] bg-white/60 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-base font-semibold">{session.name}</h3>
        {badgeForState(session.state)}
      </div>
      <p className="mt-2 text-sm text-[color:var(--muted-foreground)]">项目：{session.project}</p>
      <p className="mt-1 break-all text-sm text-[color:var(--muted-foreground)]">{session.root}</p>
    </div>
  )
}

function ProjectRow({
  project,
  submitting,
  onOpen,
  onRemove,
}: {
  project: ProjectView
  submitting: boolean
  onOpen: () => void
  onRemove: () => void
}) {
  return (
    <RowCard
      title={project.name}
      subtitle={project.root}
      badge={<Badge variant={project.dynamic ? "warning" : "default"}>{project.dynamic ? "动态项目" : "静态项目"}</Badge>}
      action={
        <div className="flex flex-wrap justify-end gap-2">
          <Button size="sm" disabled={submitting} onClick={onOpen}>
            打开
          </Button>
          {project.dynamic ? (
            <ConfirmAction
              title={`删除项目 ${project.name}？`}
              description="动态项目会从白名单中移除；如果存在对应会话，也会一起删除。"
              triggerLabel="删除项目"
              variant="destructive"
              disabled={submitting}
              onConfirm={onRemove}
            />
          ) : null}
        </div>
      }
    />
  )
}

function RowCard({
  title,
  subtitle,
  badge,
  action,
}: {
  title: string
  subtitle: string
  badge: ReactNode
  action: ReactNode
}) {
  return (
    <div className="flex flex-col gap-3 rounded-2xl border border-[color:var(--border)] bg-white/55 p-4 md:flex-row md:items-start md:justify-between">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="text-sm font-semibold">{title}</h3>
          {badge}
        </div>
        <p className="mt-2 break-all text-sm text-[color:var(--muted-foreground)]">{subtitle}</p>
      </div>
      {action}
    </div>
  )
}

function Banner({ tone, text }: { tone: "error" | "info"; text: string }) {
  return (
    <div
      className={
        tone === "error"
          ? "rounded-2xl border border-[#e9b0a4] bg-[#fff2ef] px-4 py-3 text-sm text-[#8c2818]"
          : "rounded-2xl border border-[#d7c8ae] bg-[#fff9ef] px-4 py-3 text-sm text-[color:var(--foreground)]"
      }
    >
      {text}
    </div>
  )
}

function ConfirmAction({
  title,
  description,
  triggerLabel,
  variant,
  disabled,
  onConfirm,
}: {
  title: string
  description: string
  triggerLabel: string
  variant: "destructive" | "secondary"
  disabled?: boolean
  onConfirm: () => void
}) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button disabled={disabled} size="sm" variant={variant}>
          {triggerLabel}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancelButton>取消</AlertDialogCancelButton>
          <AlertDialogActionButton onClick={onConfirm}>确认</AlertDialogActionButton>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function badgeForState(state: string) {
  if (state === "running") {
    return <Badge variant="success">running</Badge>
  }
  if (state === "starting") {
    return <Badge variant="warning">starting</Badge>
  }
  if (state === "lost" || state === "exited") {
    return <Badge variant="destructive">{state}</Badge>
  }
  return <Badge>{state}</Badge>
}

function lastPathSegment(path: string) {
  const parts = path.split("/").filter(Boolean)
  return parts[parts.length - 1] ?? ""
}
