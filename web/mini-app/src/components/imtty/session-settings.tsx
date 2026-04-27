"use client"

import { useState } from "react"
import { ChevronDown, Sliders } from "lucide-react"

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  ToggleGroup,
  ToggleGroupItem,
} from "@/components/ui/toggle-group"
import type {
  ModelId,
  ModelOption,
  PlanMode,
  Reasoning,
  Session,
} from "@/lib/imtty-types"
import {
  PLAN_MODE_OPTIONS,
  FALLBACK_MODEL_OPTIONS,
  FALLBACK_REASONING_OPTIONS,
} from "@/lib/imtty-types"

export function SessionSettings({
  session,
  modelOptions = FALLBACK_MODEL_OPTIONS,
  onChangeModel,
  onChangeReasoning,
  onChangePlanMode,
}: {
  session: Session | null
  modelOptions?: ModelOption[]
  onChangeModel: (model: ModelId) => void
  onChangeReasoning: (reasoning: Reasoning) => void
  onChangePlanMode: (mode: PlanMode) => void
}) {
  const [open, setOpen] = useState(false)

  if (!session) return null
  const availableModels = modelOptions.length ? modelOptions : FALLBACK_MODEL_OPTIONS
  const currentModel = availableModels.find((model) => model.id === session.model)
  const reasoningOptions =
    currentModel?.supportedReasoning.length
      ? currentModel.supportedReasoning.map((reasoning) => {
          const fallback = FALLBACK_REASONING_OPTIONS.find((option) => option.id === reasoning)
          return { id: reasoning, label: fallback?.label ?? reasoning }
        })
      : FALLBACK_REASONING_OPTIONS

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-lg border bg-card"
    >
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex w-full items-center justify-between px-4 py-2.5 text-left"
        >
          <span className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            <Sliders className="h-3.5 w-3.5" aria-hidden="true" />
            会话设置
          </span>
          <span className="flex items-center gap-2 text-[11px] text-muted-foreground">
            下次发送时生效
            <ChevronDown
              className={`h-4 w-4 transition-transform ${
                open ? "rotate-180" : ""
              }`}
              aria-hidden="true"
            />
          </span>
        </button>
      </CollapsibleTrigger>

      <CollapsibleContent>
        <div className="flex flex-col gap-4 border-t px-4 py-4">
          <div className="flex flex-col gap-1.5">
            <Label
              htmlFor="model-select"
              className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground"
            >
              模型
            </Label>
            <Select
              value={session.model}
              onValueChange={(v) => onChangeModel(v as ModelId)}
            >
              <SelectTrigger id="model-select" className="h-9 text-sm">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {availableModels.map((m) => (
                  <SelectItem key={m.id} value={m.id}>
                    <div className="flex flex-col">
                      <span className="text-sm">{m.label}</span>
                      <span className="text-[11px] text-muted-foreground">
                        {m.hint}
                      </span>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
              推理强度
            </Label>
            <ToggleGroup
              type="single"
              value={session.reasoning}
              onValueChange={(v) => {
                if (v) onChangeReasoning(v as Reasoning)
              }}
              variant="outline"
              size="sm"
              className="grid grid-cols-4"
            >
              {reasoningOptions.map((r) => (
                <ToggleGroupItem
                  key={r.id}
                  value={r.id}
                  className="text-xs"
                  aria-label={`推理强度 ${r.label}`}
                >
                  {r.label}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
              计划模式
            </Label>
            <ToggleGroup
              type="single"
              value={session.planMode}
              onValueChange={(v) => {
                if (v) onChangePlanMode(v as PlanMode)
              }}
              variant="outline"
              size="sm"
              className="grid grid-cols-2"
            >
              {PLAN_MODE_OPTIONS.map((p) => (
                <ToggleGroupItem
                  key={p.id}
                  value={p.id}
                  className="flex-col items-start gap-0 px-3 py-2 text-left"
                >
                  <span className="text-xs font-medium">{p.label}</span>
                  <span className="text-[10px] text-muted-foreground">
                    {p.hint}
                  </span>
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>

          <p className="text-[11px] leading-relaxed text-muted-foreground">
            修改不会影响已发送的消息，只在下次发送时应用到当前会话。
          </p>
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}
