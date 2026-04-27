"use client"

import { useEffect, useState } from "react"
import { Plus } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import type { BrowseResponse, BrowseShortcutView, DirectoryEntryView } from "@/lib/api"
import { DirectoryPicker } from "./directory-picker"

interface AddProjectProps {
  onAdd: (project: { name: string; path: string }) => void | Promise<void>
  browse: BrowseResponse | null
  browseLoading: boolean
  shortcuts: BrowseShortcutView[]
  onJump: (path: string) => void
  onGoUp: () => void
  onEnterDirectory: (directory: DirectoryEntryView) => void
}

function getBaseName(path: string) {
  const parts = path.split("/").filter(Boolean)
  return parts[parts.length - 1] ?? ""
}

export function AddProject({
  onAdd,
  browse,
  browseLoading,
  shortcuts,
  onJump,
  onGoUp,
  onEnterDirectory,
}: AddProjectProps) {
  const [name, setName] = useState("")
  const [path, setPath] = useState<string | null>(null)
  const [nameTouched, setNameTouched] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  // Auto-fill the name from the directory base name unless the user edited it.
  useEffect(() => {
    if (!nameTouched && path) {
      const base = getBaseName(path)
      if (base && base !== "/") setName(base)
    }
  }, [path, nameTouched])

  const canSubmit = name.trim().length > 0 && !!path && !submitting

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!canSubmit || !path) return
    setSubmitting(true)
    try {
      await onAdd({ name: name.trim(), path })
      setName("")
      setPath(null)
      setNameTouched(false)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <section
      aria-labelledby="add-project-heading"
      className="rounded-lg border bg-card shadow-xs"
    >
      <div className="border-b px-4 py-2.5">
        <h2
          id="add-project-heading"
          className="text-xs font-medium uppercase tracking-wide text-muted-foreground"
        >
          添加项目
        </h2>
      </div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-3 px-4 py-4">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="project-name" className="text-xs font-medium">
            项目名
          </Label>
          <Input
            id="project-name"
            value={name}
            onChange={(e) => {
              setName(e.target.value)
              setNameTouched(true)
            }}
            placeholder="选择目录后会自动填入，可修改"
            autoComplete="off"
          />
        </div>

        <div className="flex flex-col gap-1.5">
          <Label className="text-xs font-medium">项目目录</Label>
          <DirectoryPicker
            browse={browse}
            browseLoading={browseLoading}
            shortcuts={shortcuts}
            selectedPath={path}
            onJump={onJump}
            onGoUp={onGoUp}
            onEnterDirectory={onEnterDirectory}
            onSelect={setPath}
            onClear={() => setPath(null)}
          />
        </div>

        <Button type="submit" disabled={!canSubmit} className="w-full">
          <Plus className="h-4 w-4" aria-hidden="true" />
          {submitting ? "添加中..." : "添加项目"}
        </Button>
      </form>
    </section>
  )
}
