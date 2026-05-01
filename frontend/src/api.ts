import { mockSnapshot } from "./mock";
import type { PromptSearchResult, Settings, ShareServerInfo, Snapshot, Task } from "./types";

const api = () => window.go?.main?.App;

export const hasNativeAPI = () => Boolean(api());

export interface RuntimeInfo {
  source: string;
  readonly: boolean;
}

let runtimeInfoCache: RuntimeInfo | null = null;

export function initialRuntimeInfo(useDemo = false): RuntimeInfo {
  if (hasNativeAPI()) {
    return { source: useDemo ? "DEMO" : "LIVE", readonly: false };
  }
  if (useBrowserMock()) {
    return { source: "BROWSER MOCK", readonly: false };
  }
  return { source: "LAN READONLY", readonly: true };
}

export async function getRuntimeInfo(useDemo = false): Promise<RuntimeInfo> {
  if (hasNativeAPI()) {
    return { source: useDemo ? "DEMO" : "LIVE", readonly: false };
  }
  if (useBrowserMock()) {
    return { source: "BROWSER MOCK", readonly: false };
  }
  if (runtimeInfoCache) {
    return runtimeInfoCache;
  }
  try {
    const response = await fetch("/api/meta", { cache: "no-store" });
    if (response.ok) {
      const meta = await response.json() as Partial<RuntimeInfo>;
      runtimeInfoCache = {
        source: meta.source || "LAN READONLY",
        readonly: meta.readonly !== false,
      };
      return runtimeInfoCache;
    }
  } catch {
    // Fall through to mock mode when the built frontend is opened without the readonly server.
  }
  runtimeInfoCache = { source: "BROWSER MOCK", readonly: false };
  return runtimeInfoCache;
}

export async function startReadonlyServer(): Promise<ShareServerInfo | null> {
  const native = api();
  if (!native) {
    return null;
  }
  return native.StartReadonlyServer();
}

export async function getShareServer(): Promise<ShareServerInfo | null> {
  const native = api();
  if (!native) {
    return null;
  }
  const info = await native.GetShareServer();
  return info?.url ? info : null;
}

export const defaultSettings: Settings = {
  cockpit_owner: "",
  codex_home: "~/.codex",
  claude_home: "~/.claude",
  codex_bin: "codex",
  wezterm_bin: "wezterm",
  recent_window_hours: 48,
  idle_after_seconds: 600,
  stuck_after_seconds: 1200,
  summary_interval_seconds: 60,
  max_sessions: 40,
  show_unbound: true,
  enable_llm_summary: true,
  binding_mode: "auto",
  notify_attention: true,
  notify_completed: true,
  notify_stuck: true,
  notify_input_sound: true,
  notification_mode: "normal",
  quiet_hours_enabled: false,
  quiet_hours_start: "22:00",
  quiet_hours_end: "08:00",
  attention_rules: [
    { id: "permission_denied", name: "Permission denied", enabled: true, pattern: "permission denied|requires approval|operation not permitted", severity: "blocked", message: "permission or approval needed" },
    { id: "auth_required", name: "Auth required", enabled: true, pattern: "authentication failed|unauthorized|forbidden|login required", severity: "blocked", message: "authentication required" },
    { id: "merge_conflict", name: "Merge conflict", enabled: true, pattern: "merge conflict|<<<<<<<|conflict", severity: "blocked", message: "merge conflict detected" },
    { id: "tests_failed", name: "Tests failed", enabled: true, pattern: "test failed|tests failed|failing test|1 failed|failed tests", severity: "attention", message: "tests failed" },
    { id: "build_failed", name: "Build failed", enabled: true, pattern: "build failed|compilation failed|compile error", severity: "attention", message: "build failed" },
    { id: "runtime_error", name: "Runtime error", enabled: true, pattern: "error:|exception|traceback|panic:", severity: "attention", message: "error detected" },
    { id: "tool_blocked", name: "Tool blocked", enabled: true, pattern: "tool_use_error|blocked:", severity: "blocked", message: "tool blocked" },
  ],
};

export async function getSnapshot(useDemo: boolean): Promise<Snapshot> {
  const native = api();
  if (native && useDemo) {
    return native.GetDemoSnapshot();
  }
  if (native) {
    return native.GetSnapshot();
  }
  if ((await getRuntimeInfo(useDemo)).readonly) {
    return fetchReadonly<Snapshot>(`/api/snapshot${useDemo ? "?demo=true" : ""}`, "snapshot refresh failed");
  }
  return mockSnapshot();
}

export async function getSettings(): Promise<Settings> {
  const native = api();
  if (native) {
    return native.GetSettings();
  }
  if ((await getRuntimeInfo()).readonly) {
    return fetchReadonly<Settings>("/api/settings", "settings load failed");
  }
  return defaultSettings;
}

export async function saveSettings(settings: Settings): Promise<Settings> {
  const native = api();
  if (!native) {
    await requireWritable();
    return settings;
  }
  return native.SaveSettings(settings);
}

export async function openPane(paneID: number): Promise<void> {
  const native = api();
  if (!native) {
    await requireWritable();
    return;
  }
  await native.OpenPane(paneID);
}

export async function archiveTask(taskID: string): Promise<Snapshot> {
  const native = api();
  if (!native) {
    await requireWritable();
    const snap = mockSnapshot();
    snap.tasks = snap.tasks.map((task) => (task.id === taskID ? { ...task, archived: true } : task));
    return snap;
  }
  return native.ArchiveTask(taskID);
}

export async function ignoreTask(taskID: string): Promise<Snapshot> {
  const native = api();
  if (!native) {
    await requireWritable();
    const snap = mockSnapshot();
    snap.tasks = snap.tasks.map((task) => (task.id === taskID ? { ...task, ignored: true } : task));
    return snap;
  }
  return native.IgnoreTask(taskID);
}

export async function restoreTask(taskID: string): Promise<Snapshot> {
  const native = api();
  if (!native) {
    await requireWritable();
    const snap = mockSnapshot();
    snap.tasks = snap.tasks.map((task) => (task.id === taskID ? { ...task, archived: false, ignored: false } : task));
    return snap;
  }
  return native.RestoreTask(taskID);
}

export async function attachTask(taskID: string, paneID: number): Promise<Snapshot> {
  const native = api();
  if (!native) {
    await requireWritable();
    const snap = mockSnapshot();
    const pane = snap.panes.find((candidate) => candidate.pane_id === paneID);
    snap.tasks = snap.tasks.map((task) =>
      task.id === taskID
        ? {
            ...task,
            binding: {
              pane,
              confidence: pane ? 999 : 0,
              reasons: pane ? ["manual attach"] : [],
            },
          }
        : task,
    );
    return snap;
  }
  return native.AttachTask(taskID, paneID);
}

export async function detachTask(taskID: string): Promise<Snapshot> {
  const native = api();
  if (!native) {
    await requireWritable();
    const snap = mockSnapshot();
    snap.tasks = snap.tasks.map((task) =>
      task.id === taskID
        ? {
            ...task,
            binding: {
              confidence: 0,
              reasons: [],
            },
          }
        : task,
    );
    return snap;
  }
  return native.DetachTask(taskID);
}

export async function renameMission(missionID: string, name: string): Promise<Snapshot> {
  const native = api();
  if (!native) {
    await requireWritable();
    const snap = mockSnapshot();
    snap.missions = (snap.missions ?? []).map((mission) =>
      mission.id === missionID
        ? {
            ...mission,
            name: name.trim() || mission.default_name || mission.name,
            renamed: Boolean(name.trim()),
          }
        : mission,
    );
    return snap;
  }
  return native.RenameMission(missionID, name);
}

export async function searchPrompts(query: string): Promise<PromptSearchResult[]> {
  const native = api();
  if (native) {
    const results = await native.SearchPrompts(query);
    return Array.isArray(results) ? results : [];
  }
  if ((await getRuntimeInfo()).readonly) {
    const params = new URLSearchParams({ q: query });
    const results = await fetchReadonly<PromptSearchResult[] | null>(`/api/prompts/search?${params.toString()}`, "prompt search failed");
    return Array.isArray(results) ? results : [];
  }
  return searchMockPrompts(mockSnapshot(), query);
}

export async function reportFrontendError(message: string, stack = ""): Promise<void> {
  const native = api();
  if (!native?.ReportFrontendError) {
    console.error("Cockpit frontend error", message, stack);
    return;
  }
  try {
    await native.ReportFrontendError(message, stack);
  } catch (error) {
    console.error("Cockpit frontend error logging failed", error);
  }
}

export async function refreshSummary(taskID: string): Promise<Task> {
  const native = api();
  if (!native) {
    await requireWritable();
    const task = mockSnapshot().tasks.find((candidate) => candidate.id === taskID);
    if (!task) {
      throw new Error(`task ${taskID} not found`);
    }
    return { ...task, summary: `${task.summary} (refreshed)` };
  }
  return native.RefreshSummary(taskID);
}

export async function generateDebrief(taskID: string): Promise<Task> {
  const native = api();
  if (!native) {
    await requireWritable();
    const task = mockSnapshot().tasks.find((candidate) => candidate.id === taskID);
    if (!task) {
      throw new Error(`task ${taskID} not found`);
    }
    return {
      ...task,
      debrief: {
        task_id: task.id,
        session_id: task.session.id,
        generated_at: new Date().toISOString(),
        input_hash: "mock",
        text: "## Completed\nMock debrief generated from browser data.\n\n## Current State\nTask is ready for review.\n\n## Changed Files\nSee Diff Radar.\n\n## Tests\nNo native test evidence in browser mode.\n\n## Risks\nMock data only.\n\n## Next Action\nOpen the bound pane.",
      },
    };
  }
  return native.GenerateDebrief(taskID);
}

export async function reviewDoneItem(itemID: string): Promise<Snapshot> {
  const native = api();
  if (!native) {
    await requireWritable();
    const snap = mockSnapshot();
    snap.done_inbox = (snap.done_inbox ?? []).map((item) => (item.id === itemID ? { ...item, reviewed: true } : item));
    return snap;
  }
  return native.ReviewDoneItem(itemID);
}

export async function archiveDoneItem(itemID: string): Promise<Snapshot> {
  const native = api();
  if (!native) {
    await requireWritable();
    const snap = mockSnapshot();
    snap.done_inbox = (snap.done_inbox ?? []).filter((item) => item.id !== itemID);
    return snap;
  }
  return native.ArchiveDoneItem(itemID);
}

async function requireWritable() {
  if ((await getRuntimeInfo()).readonly) {
    throw new Error("readonly LAN cockpit");
  }
}

async function fetchReadonly<T>(url: string, fallback: string): Promise<T> {
  const response = await fetch(url, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(await readonlyError(response, fallback));
  }
  return response.json() as Promise<T>;
}

async function readonlyError(response: Response, fallback: string) {
  try {
    const body = await response.json() as { error?: string };
    return body.error || fallback;
  } catch {
    return fallback;
  }
}

function useBrowserMock() {
  return import.meta.env.DEV || window.location.protocol === "file:";
}

function searchMockPrompts(snapshot: Snapshot, query: string): PromptSearchResult[] {
  const cleanQuery = query.trim().toLowerCase();
  const taskMission = new Map<string, NonNullable<Snapshot["missions"]>[number]>();
  for (const mission of snapshot.missions ?? []) {
    for (const taskID of mission.task_ids) {
      taskMission.set(taskID, mission);
    }
  }
  return snapshot.tasks
    .flatMap((task) => {
      const mission = taskMission.get(task.id);
      return (task.session.events ?? [])
        .filter((event) => event.type === "user" && event.text.trim())
        .map((event, index) => {
          const prompt = event.text.trim();
          const text = prompt.toLowerCase();
          const exact = cleanQuery && text.includes(cleanQuery) ? 8 : 0;
          const tokens = cleanQuery.split(/\s+/).filter(Boolean);
          const matched = tokens.filter((token) => text.includes(token)).length;
          return {
            id: `${task.id}:${event.at ?? index}:${index}`,
            agent: task.session.agent,
            session_id: task.session.id,
            task_id: task.id,
            mission_id: mission?.id,
            mission_name: mission?.name,
            cwd: task.session.cwd,
            prompt,
            at: event.at,
            score: cleanQuery ? exact + matched * 2 : 1,
            mode: "keyword" as const,
            summary: task.summary,
          };
        });
    })
    .filter((result) => !cleanQuery || result.score > 0)
    .sort((a, b) => b.score - a.score || String(b.at ?? "").localeCompare(String(a.at ?? "")))
    .slice(0, 40);
}
