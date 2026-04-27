export type SessionStatus = "running" | "starting" | "disconnected" | "exited" | "idle"

export type ProjectKind = "fixed" | "dynamic"

export type ModelId = string

export type Reasoning = string

export type PlanMode = "default" | "plan"

export interface Project {
  id: string
  name: string
  path: string
  kind: ProjectKind
}

export interface Session {
  id: string
  name: string
  projectId: string
  projectName: string
  projectPath: string
  status: SessionStatus
  /** Codex thread id used to resume a conversation. */
  threadId: string
  /** Git branch the session is currently working on. */
  branch: string
  model: ModelId
  reasoning: Reasoning
  planMode: PlanMode
  /** Tokens used in the current context window. */
  tokenUsage: number
  /** Total context window size. */
  tokenLimit: number
}

export const STATUS_LABEL: Record<SessionStatus, string> = {
  running: "运行中",
  starting: "启动中",
  disconnected: "已断开",
  exited: "已退出",
  idle: "空闲",
}

export interface ModelOption {
  id: ModelId
  label: string
  hint: string
  supportedReasoning: string[]
}

export const FALLBACK_MODEL_OPTIONS: ModelOption[] = [
  { id: "gpt-5.5", label: "gpt-5.5", hint: "当前配置", supportedReasoning: ["medium", "high", "xhigh"] },
]

export const FALLBACK_REASONING_OPTIONS: { id: Reasoning; label: string }[] = [
  { id: "low", label: "低" },
  { id: "medium", label: "中" },
  { id: "high", label: "高" },
  { id: "xhigh", label: "极高" },
]

export const PLAN_MODE_OPTIONS: { id: PlanMode; label: string; hint: string }[] = [
  { id: "default", label: "默认", hint: "普通对话模式" },
  { id: "plan", label: "Plan", hint: "更偏规划与深度思考" },
]

export function reasoningLabel(reasoning: string) {
  const item = FALLBACK_REASONING_OPTIONS.find((option) => option.id === reasoning)
  return item?.label ?? reasoning
}
