"use client"

import { ArrowRightLeft, CircleDot } from "lucide-react"

import { Button } from "@/components/ui/button"
import type { Session } from "@/lib/imtty-types"
import { StatusBadge } from "./status-badge"

interface SessionListProps {
  sessions: Session[]
  currentSessionId: string | null
  onSwitch: (sessionId: string) => void
}

export function SessionList({ sessions, currentSessionId, onSwitch }: SessionListProps) {
  return (
    <section
      aria-labelledby="session-list-heading"
      className="rounded-lg border bg-card shadow-xs"
    >
      <div className="flex items-center justify-between border-b px-4 py-2.5">
        <h2
          id="session-list-heading"
          className="text-xs font-medium uppercase tracking-wide text-muted-foreground"
        >
          会话列表
        </h2>
        <span className="text-[11px] tabular-nums text-muted-foreground">
          {sessions.length} 个
        </span>
      </div>

      {sessions.length === 0 ? (
        <div className="px-4 py-6">
          <p className="text-sm text-foreground">当前还没有会话。</p>
          <p className="mt-1 text-xs text-muted-foreground">
            你可以先从项目列表打开一个项目。
          </p>
        </div>
      ) : (
        <ul className="divide-y">
          {sessions.map((s) => {
            const isCurrent = s.id === currentSessionId
            return (
              <li
                key={s.id}
                className="flex items-center gap-3 px-4 py-3"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <p className="truncate text-sm font-medium text-foreground">
                      {s.name}
                    </p>
                    <StatusBadge status={s.status} size="xs" />
                  </div>
                  <p
                    className="mt-0.5 truncate font-mono text-xs text-muted-foreground"
                    title={s.projectPath}
                  >
                    {s.projectPath}
                  </p>
                </div>

                {isCurrent ? (
                  <span className="inline-flex shrink-0 items-center gap-1 rounded-md bg-accent px-2 py-1 text-xs font-medium text-accent-foreground">
                    <CircleDot className="h-3 w-3" aria-hidden="true" />
                    当前会话
                  </span>
                ) : (
                  <Button
                    size="sm"
                    variant="secondary"
                    className="shrink-0"
                    onClick={() => onSwitch(s.id)}
                  >
                    <ArrowRightLeft className="h-3.5 w-3.5" aria-hidden="true" />
                    打开并切换
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
