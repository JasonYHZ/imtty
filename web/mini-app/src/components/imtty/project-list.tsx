"use client"

import { useState } from "react"
import { ExternalLink, Trash2, Lock } from "lucide-react"

import { Button } from "@/components/ui/button"
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
import type { Project } from "@/lib/imtty-types"

interface ProjectListProps {
  projects: Project[]
  onOpen: (projectId: string) => void
  onDelete: (projectId: string) => void
}

export function ProjectList({ projects, onOpen, onDelete }: ProjectListProps) {
  const [pendingDelete, setPendingDelete] = useState<Project | null>(null)

  return (
    <section
      aria-labelledby="project-list-heading"
      className="rounded-lg border bg-card shadow-xs"
    >
      <div className="flex items-center justify-between border-b px-4 py-2.5">
        <h2
          id="project-list-heading"
          className="text-xs font-medium uppercase tracking-wide text-muted-foreground"
        >
          项目列表
        </h2>
        <span className="text-[11px] tabular-nums text-muted-foreground">
          {projects.length} 个
        </span>
      </div>

      {projects.length === 0 ? (
        <div className="px-4 py-6">
          <p className="text-sm text-foreground">还没有可打开的项目。</p>
          <p className="mt-1 text-xs text-muted-foreground">
            在上方“添加项目”里把本机目录添加进来。
          </p>
        </div>
      ) : (
        <ul className="divide-y">
          {projects.map((p) => (
            <li key={p.id} className="flex items-center gap-3 px-4 py-3">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <p className="truncate text-sm font-medium text-foreground">
                    {p.name}
                  </p>
                  <span
                    className={
                      p.kind === "fixed"
                        ? "inline-flex items-center gap-1 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground"
                        : "inline-flex items-center rounded-full bg-blue-50 px-1.5 py-0.5 text-[10px] font-medium text-blue-700"
                    }
                  >
                    {p.kind === "fixed" ? (
                      <>
                        <Lock className="h-2.5 w-2.5" aria-hidden="true" />
                        固定项目
                      </>
                    ) : (
                      "动态项目"
                    )}
                  </span>
                </div>
                <p
                  className="mt-0.5 truncate font-mono text-xs text-muted-foreground"
                  title={p.path}
                >
                  {p.path}
                </p>
              </div>

              <div className="flex shrink-0 items-center gap-1">
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => onOpen(p.id)}
                  className="gap-1.5"
                >
                  <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
                  打开
                </Button>
                {p.kind === "dynamic" ? (
                  <Button
                    size="icon"
                    variant="ghost"
                    onClick={() => setPendingDelete(p)}
                    className="h-8 w-8 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                    aria-label={`删除 ${p.name}`}
                  >
                    <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                  </Button>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      )}

      <AlertDialog
        open={!!pendingDelete}
        onOpenChange={(open) => !open && setPendingDelete(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              删除项目「{pendingDelete?.name}」？
            </AlertDialogTitle>
            <AlertDialogDescription>
              删除后，这个项目会从可打开列表中移除。如果它有关联会话，也可能一起被清理。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (pendingDelete) onDelete(pendingDelete.id)
                setPendingDelete(null)
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
