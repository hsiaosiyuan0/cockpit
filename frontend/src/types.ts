export type Agent = "codex" | "claude";

export type EventType = "user" | "assistant" | "tool" | "result" | "system" | "error";

export type Status =
  | "needs_attention"
  | "waiting"
  | "working"
  | "drift"
  | "blocked"
  | "idle"
  | "completed"
  | "unbound"
  | "unknown";

export interface Event {
  at?: string;
  type: EventType;
  text: string;
  detail?: string;
}

export type TraceKind = "turn" | "assistant" | "tool" | "subagent";

export type TraceStatus = "running" | "succeeded" | "failed" | "unknown";

export interface TraceSpan {
  id: string;
  parent_id?: string;
  kind: TraceKind;
  name: string;
  status: TraceStatus;
  started_at?: string;
  ended_at?: string;
  duration_millis?: number;
  summary?: string;
  detail?: string;
  exit_code?: number;
  children?: TraceSpan[];
}

export interface TraceRef {
  id: string;
  kind: TraceKind;
  name: string;
  started_at?: string;
  summary?: string;
  detail?: string;
}

export interface StatusExplanation {
  status: Status;
  rule?: string;
  rule_severity?: string;
  reason?: string;
  why_not_working?: string;
  why_not_idle?: string;
  latest_assistant_at?: string;
  last_event_at?: string;
  active_trace?: TraceRef;
  background_run_traces?: TraceRef[];
  attention_event?: Event;
  evidence?: string[];
}

export interface DiffFile {
  path: string;
  status: string;
  additions?: number;
  deletions?: number;
}

export interface DiffRadar {
  repo_root?: string;
  branch?: string;
  dirty: boolean;
  files?: DiffFile[];
  added: number;
  modified: number;
  deleted: number;
  renamed: number;
  untracked: number;
  tests: number;
  lockfiles: number;
  summary?: string;
  last_error?: string;
  generated_at?: string;
}

export interface FlightEvent {
  id: string;
  parent_id?: string;
  at?: string;
  kind: string;
  title: string;
  detail?: string;
  status?: string;
  duration_millis?: number;
  source?: string;
}

export interface Debrief {
  task_id: string;
  session_id: string;
  generated_at: string;
  text: string;
  input_hash: string;
  raw?: string;
}

export interface PromptSearchResult {
  id: string;
  agent: Agent;
  session_id: string;
  task_id: string;
  mission_id?: string;
  mission_name?: string;
  cwd?: string;
  prompt: string;
  at?: string;
  score: number;
  mode: "keyword";
  summary?: string;
}

export interface Session {
  id: string;
  agent: Agent;
  cwd?: string;
  log_path?: string;
  title?: string;
  last_prompt?: string;
  created_at?: string;
  last_event_at?: string;
  pid?: number;
  process_uuid?: string;
  internal?: boolean;
  internal_reason?: string;
  events?: Event[];
  trace?: TraceSpan[];
}

export interface Pane {
  window_id: number;
  tab_id: number;
  pane_id: number;
  workspace?: string;
  title?: string;
  tab_title?: string;
  window_title?: string;
  cwd?: string;
  tty_name?: string;
  is_active?: boolean;
}

export interface Process {
  pid: number;
  ppid: number;
  tty?: string;
  args?: string;
}

export interface Binding {
  pane?: Pane;
  process?: Process;
  confidence: number;
  reasons?: string[];
}

export interface Task {
  id: string;
  session: Session;
  binding: Binding;
  status: Status;
  status_explanation?: StatusExplanation;
  attention_reason?: string;
  summary?: string;
  summary_at?: string;
  diff?: DiffRadar;
  flight?: FlightEvent[];
  debrief?: Debrief;
  archived?: boolean;
  ignored?: boolean;
}

export interface DoneInboxItem {
  id: string;
  task_id: string;
  session_id: string;
  agent: Agent;
  project: string;
  cwd?: string;
  title?: string;
  status: Status;
  summary?: string;
  completed_at?: string;
  reviewed?: boolean;
  archived?: boolean;
  pane_id?: number;
  diff?: DiffRadar;
  debrief?: Debrief;
}

export interface Mission {
  id: string;
  name: string;
  default_name?: string;
  renamed?: boolean;
  cwd?: string;
  repo_root?: string;
  status: Status;
  task_ids: string[];
  attention: number;
  working: number;
  drift: number;
  blocked: number;
  idle: number;
  completed: number;
  changed_files: number;
  last_event_at?: string;
  summary?: string;
}

export interface Snapshot {
  built_at: string;
  tasks: Task[];
  panes: Pane[];
  processes: Process[];
  missions?: Mission[];
  done_inbox?: DoneInboxItem[];
  errors?: string[];
}

export interface AttentionRule {
  id: string;
  name: string;
  enabled: boolean;
  pattern: string;
  severity: "attention" | "blocked" | "drift";
  scope?: string;
  message?: string;
}

export interface Settings {
  cockpit_owner?: string;
  codex_home: string;
  claude_home: string;
  codex_bin: string;
  wezterm_bin: string;
  recent_window_hours: number;
  idle_after_seconds: number;
  stuck_after_seconds: number;
  summary_interval_seconds: number;
  max_sessions: number;
  show_unbound: boolean;
  enable_llm_summary: boolean;
  binding_mode: "auto" | "manual";
  notify_attention: boolean;
  notify_completed: boolean;
  notify_stuck: boolean;
  notification_mode: "normal" | "focus" | "silent";
  quiet_hours_enabled: boolean;
  quiet_hours_start: string;
  quiet_hours_end: string;
  attention_rules?: AttentionRule[];
}

export interface CockpitAPI {
  GetSnapshot(): Promise<Snapshot>;
  GetDemoSnapshot(): Promise<Snapshot>;
  GetLogPath(): Promise<string>;
  GetSettings(): Promise<Settings>;
  GetShareServer(): Promise<ShareServerInfo>;
  StartReadonlyServer(): Promise<ShareServerInfo>;
  SaveSettings(settings: Settings): Promise<Settings>;
  OpenPane(paneID: number): Promise<void>;
  ArchiveTask(taskID: string): Promise<Snapshot>;
  IgnoreTask(taskID: string): Promise<Snapshot>;
  RestoreTask(taskID: string): Promise<Snapshot>;
  AttachTask(taskID: string, paneID: number): Promise<Snapshot>;
  DetachTask(taskID: string): Promise<Snapshot>;
  RenameMission(missionID: string, name: string): Promise<Snapshot>;
  SearchPrompts(query: string): Promise<PromptSearchResult[]>;
  ReportFrontendError(message: string, stack: string): Promise<void>;
  RefreshSummary(taskID: string): Promise<Task>;
  GenerateDebrief(taskID: string): Promise<Task>;
  ReviewDoneItem(itemID: string): Promise<Snapshot>;
  ArchiveDoneItem(itemID: string): Promise<Snapshot>;
}

export interface ShareServerInfo {
  url: string;
  listen_addr: string;
  readonly: boolean;
}

declare global {
  interface Window {
    go?: {
      main?: {
        App?: CockpitAPI;
      };
    };
  }
}
