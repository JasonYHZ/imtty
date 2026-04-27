"use client"

import { useState } from "react"
import {
  Power,
  Trash2,
  Folder,
  GitBranch,
  Cpu,
  Brain,
  Sparkles,
  Eraser,
  Hash,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { Progress } from "@/components/ui/progress"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import type { Session } from "@/lib/imtty-types"
import { reasoningLabel } from "@/lib/imtty-types"
import { StatusBadge } from "./status-badge"

const PLAN_MODE_LABEL: Record<Session["planMode"], string> = {
  default: "默认",
  plan: "Plan",
  custom: "自定义",
}

function formatTokens(value: number) {
  if (value >= 1_000_000) return `${Math.round(value / 1_000_000)}M`
  if (value >= 1_000) return `${Math.round(value / 1_000)}K`
  return `${value}`
}

function DetailRow({
  icon: Icon,
  label,
  value,
  mono = false,
}: {
  icon: React.ComponentType<{ className?: string; "aria-hidden"?: boolean }>
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="flex items-center justify-between gap-3 py-1.5">
      <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <Icon className="h-3.5 w-3.5 shrink-0" aria-hidden={true} />
        {label}
      </span>
      <span
        className={`truncate text-right text-xs text-foreground ${
          mono ? "font-mono" : "font-medium"
        }`}
        title={value}
      >
        {value}
      </span>
    </div>
  )
}

function pendingSummary(session: Session) {
  const items: string[] = []
  if (session.pendingModel) {
    items.push(`模型 ${session.pendingModel}`)
  }
  if (session.pendingReasoning) {
    items.push(`推理 ${reasoningLabel(session.pendingReasoning)}`)
  }
  if (session.pendingPlanMode) {
    items.push(`模式 ${PLAN_MODE_LABEL[session.pendingPlanMode]}`)
  }
  return items.join(" / ")
}

export function CurrentSession({
  session,
  onClose,
  onDelete,
  onClearContext,
}: {
  session: Session | null
  onClose: () => void
  onDelete: () => void
  onClearContext: () => void
}) {
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)
  const [confirmClearOpen, setConfirmClearOpen] = useState(false)

  if (!session) {
    return (
      <section
        aria-labelledby="current-session-heading"
        className="rounded-lg border border-dashed bg-card p-5"
      >
        <h2
          id="current-session-heading"
          className="text-xs font-medium uppercase tracking-wide text-muted-foreground"
        >
          当前会话
        </h2>
        <div className="mt-3 flex flex-col items-start gap-1">
          <p className="text-sm font-medium text-foreground">没有正在使用的会话</p>
          <p className="text-xs text-muted-foreground">
            从下方项目列表打开一个项目，或先创建一个新项目。
          </p>
        </div>
      </section>
    )
  }

  const tokenPct = Math.min(
    100,
    Math.round((session.tokenUsage / session.tokenLimit) * 100),
  )
  const tokenWarn = tokenPct >= 80

  return (
    <section
      aria-labelledby="current-session-heading"
      className="rounded-lg border bg-card shadow-xs"
    >
      <div className="flex items-center justify-between border-b px-4 py-2.5">
        <h2
          id="current-session-heading"
          className="text-xs font-medium uppercase tracking-wide text-muted-foreground"
        >
          当前会话
        </h2>
        <StatusBadge status={session.status} />
      </div>

      <div className="px-4 py-4">
        <div className="flex flex-col gap-1">
          <p className="text-base font-semibold leading-tight text-foreground text-balance">
            {session.name}
          </p>
          <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <Folder className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            <span className="font-medium text-foreground">{session.projectName}</span>
          </div>
          <p
            className="break-all font-mono text-xs leading-relaxed text-muted-foreground"
            title={session.projectPath}
          >
            {session.projectPath}
          </p>
        </div>

        <div className="mt-4 divide-y rounded-md border bg-muted/30">
          <div className="grid grid-cols-2 divide-x">
            <div className="px-3 py-1.5">
              <DetailRow icon={Cpu} label="当前模型" value={session.effectiveModel || "未读取"} />
            </div>
            <div className="px-3 py-1.5">
              <DetailRow
                icon={Brain}
                label="当前推理"
                value={reasoningLabel(session.effectiveReasoning)}
              />
            </div>
          </div>
          <div className="grid grid-cols-2 divide-x">
            <div className="px-3 py-1.5">
              <DetailRow
                icon={Sparkles}
                label="当前模式"
                value={PLAN_MODE_LABEL[session.effectivePlanMode]}
              />
            </div>
            <div className="px-3 py-1.5">
              <DetailRow icon={GitBranch} label="分支" value={session.branch} mono />
            </div>
          </div>
          <div className="px-3 py-1.5">
            <DetailRow icon={Hash} label="Thread" value={session.threadId} mono />
          </div>
        </div>

        {session.hasPendingControls ? (
          <div className="mt-3 rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-xs leading-relaxed text-blue-800">
            <span className="font-medium">待生效：</span>
            {pendingSummary(session) || "已设置新的会话参数"}
            <span className="block text-blue-700/80">下一条发送给 Codex 的消息会应用这些设置。</span>
          </div>
        ) : null}

        <div className="mt-3 flex flex-col gap-1.5">
          <div className="flex items-center justify-between text-[11px]">
            <span className="text-muted-foreground">上下文窗口</span>
            <span
              className={`font-mono ${
                tokenWarn ? "text-amber-600" : "text-muted-foreground"
              }`}
            >
              {formatTokens(session.tokenUsage)} / {formatTokens(session.tokenLimit)}
              <span className="ml-1 text-foreground/60">({tokenPct}%)</span>
            </span>
          </div>
          <Progress value={tokenPct} className="h-1.5" />
        </div>

        <div className="mt-4 grid grid-cols-2 gap-2">
          <Button onClick={onClose} variant="default" className="col-span-2">
            <Power className="h-4 w-4" aria-hidden="true" />
            关闭当前会话
          </Button>
          <Button
            onClick={() => setConfirmClearOpen(true)}
            variant="outline"
            size="sm"
          >
            <Eraser className="h-4 w-4" aria-hidden="true" />
            清空上下文
          </Button>
          <Button
            onClick={() => setConfirmDeleteOpen(true)}
            variant="ghost"
            size="sm"
            className="text-destructive hover:bg-destructive/10 hover:text-destructive"
          >
            <Trash2 className="h-4 w-4" aria-hidden="true" />
            彻底删除
          </Button>
        </div>
      </div>

      <AlertDialog open={confirmClearOpen} onOpenChange={setConfirmClearOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>清空当前会话上下文？</AlertDialogTitle>
            <AlertDialogDescription>
              会话还在，但之前所有对话历史都会被清空，相当于在这个项目里开始一段全新的对话。底层会话不会重启。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                onClearContext()
                setConfirmClearOpen(false)
              }}
            >
              清空上下文
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={confirmDeleteOpen} onOpenChange={setConfirmDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>彻底删除当前会话？</AlertDialogTitle>
            <AlertDialogDescription>
              这会停止当前会话，并从列表里移除。聊天窗口不会继续连接到这个会话。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                onDelete()
                setConfirmDeleteOpen(false)
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}
