"use client"

import { ChevronLeft, Folder, FolderOpen, X, Check } from "lucide-react"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import type { BrowseResponse, BrowseShortcutView, DirectoryEntryView } from "@/lib/api"

interface DirectoryPickerProps {
  browse: BrowseResponse | null
  browseLoading: boolean
  shortcuts: BrowseShortcutView[]
  selectedPath: string | null
  onJump: (path: string) => void
  onGoUp: () => void
  onEnterDirectory: (directory: DirectoryEntryView) => void
  onSelect: (path: string) => void
  onClear: () => void
}

export function DirectoryPicker({
  browse,
  browseLoading,
  shortcuts,
  selectedPath,
  onJump,
  onGoUp,
  onEnterDirectory,
  onSelect,
  onClear,
}: DirectoryPickerProps) {
  const currentPath = browse?.current_absolute_path ?? ""
  return (
    <div className="overflow-hidden rounded-md border bg-background">
      {/* Path bar */}
      <div className="flex items-center gap-1 border-b bg-muted/40 px-2 py-1.5">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0"
          onClick={onGoUp}
          disabled={!browse?.parent_absolute_path || browseLoading}
          aria-label="上一级"
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <div
          className="min-w-0 flex-1 truncate font-mono text-xs text-foreground"
          title={currentPath}
        >
          {currentPath || "正在准备目录浏览器…"}
        </div>
      </div>

      {/* Shortcuts */}
      <div className="flex flex-wrap gap-1.5 border-b px-2 py-2">
        {shortcuts.map((s) => {
          const active = s.path === currentPath
          return (
            <button
              key={s.path}
              type="button"
              onClick={() => onJump(s.path)}
              className={cn(
                "inline-flex items-center rounded-full border px-2.5 py-1 text-xs transition-colors",
                active
                  ? "border-primary bg-primary/10 text-primary"
                  : "border-border bg-card text-muted-foreground hover:bg-muted hover:text-foreground",
              )}
            >
              {s.name}
            </button>
          )
        })}
      </div>

      {/* Children */}
      <div className="max-h-56 overflow-y-auto">
        {browseLoading ? (
          <ul className="divide-y" aria-hidden="true">
            {[0, 1, 2].map((i) => (
              <li key={i} className="flex items-center gap-2 px-3 py-2.5">
                <div className="h-4 w-4 animate-pulse rounded bg-muted" />
                <div className="h-3 w-32 animate-pulse rounded bg-muted" />
              </li>
            ))}
          </ul>
        ) : !browse ? (
          <div className="px-3 py-6 text-center text-xs text-muted-foreground">
            正在初始化目录选择器
          </div>
        ) : browse.directories.length === 0 ? (
          <div className="px-3 py-6 text-center text-xs text-muted-foreground">
            当前目录没有子目录
          </div>
        ) : (
          <ul className="divide-y">
            {browse.directories.map((directory) => {
              return (
                <li key={directory.absolute_path}>
                  <button
                    type="button"
                    onClick={() => onEnterDirectory(directory)}
                    className="flex w-full items-center gap-2 px-3 py-2.5 text-left text-sm hover:bg-muted"
                  >
                    <Folder className="h-4 w-4 shrink-0 text-muted-foreground" />
                    <span className="truncate">{directory.name}</span>
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </div>

      {/* Footer: select / clear */}
      <div className="flex items-center justify-between gap-2 border-t bg-muted/40 px-2 py-2">
        <Button
          type="button"
          size="sm"
          variant="secondary"
          onClick={() => currentPath && onSelect(currentPath)}
          disabled={!currentPath || browseLoading}
          className="gap-1.5"
        >
          <Check className="h-3.5 w-3.5" />
          选择当前目录
        </Button>
        {selectedPath ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={onClear}
            className="text-muted-foreground"
          >
            <X className="h-3.5 w-3.5" />
            清空已选
          </Button>
        ) : null}
      </div>

      {/* Selected indicator */}
      {selectedPath ? (
        <div className="flex items-start gap-2 border-t bg-accent/40 px-3 py-2">
          <FolderOpen
            className="mt-0.5 h-4 w-4 shrink-0 text-accent-foreground"
            aria-hidden="true"
          />
          <div className="min-w-0 flex-1">
            <p className="text-[11px] font-medium uppercase tracking-wide text-accent-foreground/80">
              已选目录
            </p>
            <p
              className="break-all font-mono text-xs text-accent-foreground"
              title={selectedPath}
            >
              {selectedPath}
            </p>
          </div>
        </div>
      ) : null}
    </div>
  )
}
