import {
  Archive,
  BellRing,
  Check,
  CheckCircle2,
  CircleDot,
  ClipboardCheck,
  Command,
  Copy,
  Database,
  Pencil,
  Eye,
  EyeOff,
  FileText,
  GitBranch,
  Inbox,
  LoaderCircle,
  Maximize2,
  Minimize2,
  MonitorUp,
  PanelRightOpen,
  Radar,
  RefreshCw,
  RotateCcw,
  Search,
  ShieldAlert,
  SatelliteDish,
  Settings as SettingsIcon,
  Sparkles,
  X,
  XCircle,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  attachTask,
  archiveTask,
  archiveDoneItem,
  defaultSettings,
  detachTask,
  generateDebrief,
  getSettings,
  getRuntimeInfo,
  getShareServer,
  getSnapshot,
  hasNativeAPI,
  ignoreTask,
  initialRuntimeInfo,
  openPane,
  refreshSummary,
  renameMission,
  reportFrontendError,
  reviewDoneItem,
  restoreTask,
  saveSettings,
  searchPrompts,
  startReadonlyServer,
} from "./api";
import type { RuntimeInfo } from "./api";
import type {
  AttentionRule,
  DiffRadar,
  DoneInboxItem,
  Event as SessionEvent,
  FlightEvent,
  Mission,
  Pane,
  PromptSearchResult,
  Settings as CockpitSettings,
  Snapshot,
  Status,
  StatusExplanation,
  Task,
  TraceRef,
  TraceSpan,
  TraceStatus,
} from "./types";

const lanes: Array<{ status: Status; title: string }> = [
  { status: "needs_attention", title: "needs attention" },
  { status: "waiting", title: "waiting" },
  { status: "blocked", title: "blocked" },
  { status: "drift", title: "drift" },
  { status: "working", title: "working" },
  { status: "idle", title: "idle" },
  { status: "unbound", title: "unbound" },
  { status: "completed", title: "completed" },
];

const statusLabel: Record<Status, string> = {
  needs_attention: "ATTN",
  waiting: "WAIT",
  blocked: "BLKD",
  drift: "DRFT",
  working: "WORK",
  idle: "IDLE",
  completed: "DONE",
  unbound: "LINK",
  unknown: "UNKN",
};

type ExpandablePanel = "diff" | "trace" | "status" | "debrief" | "flight";
type TraceFilter = "active" | "failed" | "tools" | "assistant" | "all";
type NumericSettingKey = "recent_window_hours" | "idle_after_seconds" | "stuck_after_seconds" | "summary_interval_seconds" | "max_sessions";
type BoardMode = "sessions" | "missions";
type InspectorTab = "overview" | ExpandablePanel;

const traceFilterOptions: Array<{ id: TraceFilter; label: string }> = [
  { id: "active", label: "active" },
  { id: "failed", label: "failed" },
  { id: "tools", label: "tools" },
  { id: "assistant", label: "assistant" },
  { id: "all", label: "all" },
];

const inspectorTabs: Array<{ id: InspectorTab; label: string }> = [
  { id: "overview", label: "Overview" },
  { id: "diff", label: "Diff" },
  { id: "trace", label: "Trace" },
  { id: "status", label: "Status" },
  { id: "flight", label: "Flight" },
  { id: "debrief", label: "Debrief" },
];

function App() {
  const nativeRuntime = hasNativeAPI();
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [selectedID, setSelectedID] = useState<string>("");
  const [useDemo, setUseDemo] = useState(false);
  const [loading, setLoading] = useState(false);
  const [refreshingID, setRefreshingID] = useState("");
  const [debriefingID, setDebriefingID] = useState("");
  const [showHidden, setShowHidden] = useState(false);
  const [message, setMessage] = useState("warming up instruments");
  const [clock, setClock] = useState(() => new Date());
  const [boardMode, setBoardMode] = useState<BoardMode>("sessions");
  const [inboxOpen, setInboxOpen] = useState(false);
  const [commandOpen, setCommandOpen] = useState(false);
  const [promptSearchOpen, setPromptSearchOpen] = useState(false);
  const [ambientOpen, setAmbientOpen] = useState(false);
  const [launchingID, setLaunchingID] = useState("");
  const [splitPercent, setSplitPercent] = useState(() => {
    const stored = window.localStorage.getItem("cockpit.splitPercent");
    const parsed = stored ? Number(stored) : 54;
    return Number.isFinite(parsed) ? Math.min(68, Math.max(38, parsed)) : 54;
  });
  const [expandedPanel, setExpandedPanel] = useState<ExpandablePanel | null>(null);
  const [closingPanel, setClosingPanel] = useState<ExpandablePanel | null>(null);
  const [settings, setSettings] = useState<CockpitSettings>(defaultSettings);
  const [settingsLoaded, setSettingsLoaded] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [savingSettings, setSavingSettings] = useState(false);
  const [runtimeInfo, setRuntimeInfo] = useState<RuntimeInfo>(() => initialRuntimeInfo(useDemo));
  const [shareURL, setShareURL] = useState("");
  const [sharing, setSharing] = useState(false);
  const [webWelcomeOpen, setWebWelcomeOpen] = useState(false);
  const loadInFlight = useRef(false);
  const showUnbound = settings.show_unbound;
  const readonly = runtimeInfo.readonly;
  const cockpitOwner = settings.cockpit_owner?.trim() ?? "";
  const welcomeOwnerKey = cockpitOwner || "__anonymous__";
  const snapshotRefreshInterval = nativeRuntime ? 10000 : 5000;

  const load = useCallback(async () => {
    if (loadInFlight.current) {
      return;
    }
    loadInFlight.current = true;
    setLoading(true);
    try {
      const next = await getSnapshot(useDemo);
      setSnapshot(next);
      setMessage(`${displayTasks(next, showHidden, showUnbound).length} visible / ${hiddenCount(next)} hidden`);
      setSelectedID((current) => {
        if (current && displayTasks(next, showHidden, showUnbound).some((task) => task.id === current)) {
          return current;
        }
        return displayTasks(next, showHidden, showUnbound)[0]?.id ?? "";
      });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "snapshot refresh failed");
    } finally {
      loadInFlight.current = false;
      setLoading(false);
    }
  }, [showHidden, showUnbound, useDemo]);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), snapshotRefreshInterval);
    return () => window.clearInterval(timer);
  }, [load, snapshotRefreshInterval]);

  useEffect(() => {
    let cancelled = false;
    getSettings()
      .then((next) => {
        if (!cancelled) {
          setSettings(next);
          setSettingsLoaded(true);
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setMessage(error instanceof Error ? error.message : "settings load failed");
          setSettingsLoaded(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    getRuntimeInfo(useDemo)
      .then((next) => {
        if (!cancelled) {
          setRuntimeInfo(next);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setRuntimeInfo(initialRuntimeInfo(useDemo));
        }
      });
    return () => {
      cancelled = true;
    };
  }, [useDemo]);

  useEffect(() => {
    if (!hasNativeAPI()) {
      return;
    }
    let cancelled = false;
    getShareServer()
      .then((info) => {
        if (!cancelled && info?.url) {
          setShareURL(info.url);
        }
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (nativeRuntime || !settingsLoaded) {
      return;
    }
    const dismissed = window.localStorage.getItem("cockpit.webWelcomeDismissed") === "1";
    const dismissedOwner = window.localStorage.getItem("cockpit.webWelcomeDismissedOwner") || "";
    if (!dismissed || dismissedOwner !== welcomeOwnerKey) {
      setWebWelcomeOpen(true);
    }
  }, [nativeRuntime, settingsLoaded, welcomeOwnerKey]);

  useEffect(() => {
    const timer = window.setInterval(() => setClock(new Date()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCommandOpen(true);
      }
      if ((event.metaKey || event.ctrlKey) && event.shiftKey && event.key.toLowerCase() === "f") {
        event.preventDefault();
        setPromptSearchOpen(true);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  const openPanel = useCallback((panel: ExpandablePanel) => {
    setClosingPanel(null);
    setExpandedPanel(panel);
  }, []);

  const closePanel = useCallback(() => {
    if (!expandedPanel) {
      return;
    }
    const panel = expandedPanel;
    setExpandedPanel(null);
    setClosingPanel(panel);
    window.setTimeout(() => {
      setClosingPanel((closing) => (closing === panel ? null : closing));
    }, 160);
  }, [expandedPanel]);

  useEffect(() => {
    if (!expandedPanel) {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        closePanel();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [closePanel, expandedPanel]);

  const tasks = useMemo(() => (snapshot ? displayTasks(snapshot, showHidden, showUnbound) : []), [showHidden, showUnbound, snapshot]);
  const selected = useMemo(() => tasks.find((task) => task.id === selectedID) ?? tasks[0], [selectedID, tasks]);
  const stats = useMemo(() => collectStats(snapshot, showUnbound), [showUnbound, snapshot]);
  const doneInbox = snapshot?.done_inbox ?? [];
  const unreadDoneCount = doneInbox.filter((item) => !item.reviewed).length;
  const source = hasNativeAPI() ? (useDemo ? "DEMO" : "LIVE") : runtimeInfo.source;
  const visiblePanel = expandedPanel ?? closingPanel;

  const replaceSnapshot = (next: Snapshot) => {
    setSnapshot(next);
    setMessage(`${displayTasks(next, showHidden, showUnbound).length} visible / ${hiddenCount(next)} hidden`);
  };

  const blockReadonlyAction = () => {
    setMessage("readonly LAN cockpit");
  };

  const dismissWebWelcome = useCallback(() => {
    window.localStorage.setItem("cockpit.webWelcomeDismissed", "1");
    window.localStorage.setItem("cockpit.webWelcomeDismissedOwner", welcomeOwnerKey);
    setWebWelcomeOpen(false);
  }, [welcomeOwnerKey]);

  const handleOpen = async (task: Task | undefined) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    const paneID = task?.binding.pane?.pane_id;
    if (paneID === undefined) {
      setMessage("selected task is not bound to a pane");
      return;
    }
    setLaunchingID(task?.id ?? "");
    try {
      await openPane(paneID);
      setMessage(`activated pane:${paneID}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "pane activation failed");
    } finally {
      window.setTimeout(() => setLaunchingID((current) => (current === task?.id ? "" : current)), 520);
    }
  };

  const startSplitResize = (event: React.PointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    const grid = event.currentTarget.parentElement;
    if (!grid) {
      return;
    }
    const rect = grid.getBoundingClientRect();
    const handleMove = (moveEvent: PointerEvent) => {
      const next = ((moveEvent.clientX - rect.left) / rect.width) * 100;
      const clamped = Math.min(68, Math.max(38, next));
      setSplitPercent(clamped);
      window.localStorage.setItem("cockpit.splitPercent", String(Math.round(clamped)));
    };
    const handleUp = () => {
      window.removeEventListener("pointermove", handleMove);
      window.removeEventListener("pointerup", handleUp);
    };
    window.addEventListener("pointermove", handleMove);
    window.addEventListener("pointerup", handleUp);
  };

  const handleArchive = async (task: Task | undefined) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    if (!task) {
      return;
    }
    try {
      replaceSnapshot(await archiveTask(task.id));
      setMessage(`archived ${shortID(task)}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "archive failed");
    }
  };

  const handleIgnore = async (task: Task | undefined) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    if (!task) {
      return;
    }
    try {
      replaceSnapshot(await ignoreTask(task.id));
      setMessage(`ignored ${shortID(task)}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "ignore failed");
    }
  };

  const handleRestore = async (task: Task | undefined) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    if (!task) {
      return;
    }
    try {
      replaceSnapshot(await restoreTask(task.id));
      setMessage(`restored ${shortID(task)}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "restore failed");
    }
  };

  const handleAttach = async (task: Task | undefined, paneID: number) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    if (!task) {
      return;
    }
    if (!Number.isFinite(paneID) || paneID < 0) {
      setMessage("select a valid pane");
      return;
    }
    try {
      replaceSnapshot(await attachTask(task.id, paneID));
      setMessage(`attached ${shortID(task)} to pane:${paneID}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "attach failed");
    }
  };

  const handleDetach = async (task: Task | undefined) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    if (!task) {
      return;
    }
    try {
      replaceSnapshot(await detachTask(task.id));
      setMessage(`cleared manual binding for ${shortID(task)}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "detach failed");
    }
  };

  const handleRenameMission = async (missionID: string, name: string) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    try {
      replaceSnapshot(await renameMission(missionID, name));
      setMessage(name.trim() ? "mission renamed" : "mission name reset");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "mission rename failed");
    }
  };

  const handleSummary = async (task: Task | undefined) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    if (!task) {
      return;
    }
    if (!settings.enable_llm_summary) {
      setMessage("LLM summary is disabled in settings");
      return;
    }
    setRefreshingID(task.id);
    try {
      const refreshed = await refreshSummary(task.id);
      setSnapshot((current) => {
        if (!current) {
          return current;
        }
        return {
          ...current,
          tasks: current.tasks.map((candidate) => (candidate.id === refreshed.id ? refreshed : candidate)),
        };
      });
      setMessage(`summary refreshed for ${shortID(task)}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "summary refresh failed");
    } finally {
      setRefreshingID("");
    }
  };

  const handleDebrief = async (task: Task | undefined) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    if (!task) {
      return;
    }
    if (!settings.enable_llm_summary) {
      setMessage("LLM summary is disabled in settings");
      return;
    }
    setDebriefingID(task.id);
    try {
      const refreshed = await generateDebrief(task.id);
      setSnapshot((current) => {
        if (!current) {
          return current;
        }
        return {
          ...current,
          tasks: current.tasks.map((candidate) => (candidate.id === refreshed.id ? refreshed : candidate)),
        };
      });
      setMessage(`debrief generated for ${shortID(task)}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "debrief failed");
    } finally {
      setDebriefingID("");
    }
  };

  const handleReviewDone = async (item: DoneInboxItem) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    try {
      replaceSnapshot(await reviewDoneItem(item.id));
      setMessage(`reviewed ${item.project}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "review failed");
    }
  };

  const handleArchiveDone = async (item: DoneInboxItem) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    try {
      replaceSnapshot(await archiveDoneItem(item.id));
      setMessage(`archived done item ${item.project}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "done archive failed");
    }
  };

  const selectTaskID = (taskID: string) => {
    if (!taskID) {
      return;
    }
    setSelectedID(taskID);
  };

  const handleSaveSettings = async (next: CockpitSettings) => {
    if (readonly) {
      blockReadonlyAction();
      return;
    }
    setSavingSettings(true);
    try {
      const saved = await saveSettings(next);
      setSettings(saved);
      setSettingsOpen(false);
      setMessage("settings saved");
      void load();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "settings save failed");
    } finally {
      setSavingSettings(false);
    }
  };

  const handleStartShare = async () => {
    if (!hasNativeAPI()) {
      return;
    }
    setSharing(true);
    try {
      const info = await startReadonlyServer();
      if (!info) {
        setMessage("readonly dashboard is unavailable outside the desktop app");
        return;
      }
      setShareURL(info.url);
      setMessage(`readonly dashboard ${info.url}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "readonly dashboard start failed");
    } finally {
      setSharing(false);
    }
  };

  return (
    <>
      <main className={`shell ${nativeRuntime ? "native-shell" : "web-shell"}`}>
        {nativeRuntime ? <div className="window-chrome" aria-hidden="true" /> : null}
        <header className="topbar">
        <div className="brand">
          <div className="brand-mark" aria-hidden="true">
            <BrandGauge />
          </div>
          <div>
            <h1>COCKPIT</h1>
            <span>agent flight deck</span>
          </div>
        </div>
        <div className="toolbar">
          <button className="icon-button" type="button" title="Refresh" onClick={() => void load()}>
            <RefreshCw size={16} className={loading ? "spin" : ""} />
          </button>
          <button className="icon-button badge-button" type="button" title="Done inbox" onClick={() => setInboxOpen(true)}>
            <Inbox size={16} />
            {unreadDoneCount > 0 ? <span>{unreadDoneCount}</span> : null}
          </button>
          <button className="icon-button" type="button" title="Command palette" onClick={() => setCommandOpen(true)}>
            <Command size={16} />
          </button>
          <button className="icon-button" type="button" title="Search past prompts" onClick={() => setPromptSearchOpen(true)}>
            <Search size={16} />
          </button>
          <button className={`icon-button ${ambientOpen ? "active" : ""}`} type="button" title="Ambient mode" onClick={() => setAmbientOpen(true)}>
            <Sparkles size={16} />
          </button>
          <button
            className={`seg-button ${boardMode === "missions" ? "active" : ""}`}
            type="button"
            title="Toggle mission grouping"
            onClick={() => setBoardMode((mode) => (mode === "sessions" ? "missions" : "sessions"))}
          >
            <Radar size={15} />
            <span>{boardMode.toUpperCase()}</span>
          </button>
          {hasNativeAPI() ? (
            <button
              className={`icon-button ${shareURL ? "active" : ""}`}
              type="button"
              title={shareURL ? `Readonly LAN dashboard: ${shareURL}` : "Start readonly LAN dashboard"}
              disabled={sharing}
              onClick={() => void handleStartShare()}
            >
              <SatelliteDish size={16} className={sharing ? "pulse-icon" : ""} />
            </button>
          ) : readonly ? (
            <div className="readonly-chip" title="Readonly LAN dashboard">
              <Eye size={13} />
              <span>READONLY</span>
            </div>
          ) : null}
          {shareURL ? (
            <div className="share-chip" title={shareURL}>
              <span>LAN</span>
              <strong>{shareURL.replace(/^https?:\/\//, "")}</strong>
            </div>
          ) : null}
          <button
            className={`seg-button ${useDemo ? "active" : ""}`}
            type="button"
            title="Toggle demo data"
            onClick={() => setUseDemo((value) => !value)}
          >
            <Database size={15} />
            <span>{source}</span>
          </button>
          <button
            className={`icon-button ${showHidden ? "active" : ""}`}
            type="button"
            title={showHidden ? "Hide archived and ignored tasks" : "Show archived and ignored tasks"}
            onClick={() => setShowHidden((value) => !value)}
          >
            {showHidden ? <Eye size={16} /> : <EyeOff size={16} />}
          </button>
          <button className="icon-button" type="button" title={readonly ? "Readonly LAN view" : "Settings"} disabled={readonly} onClick={() => setSettingsOpen(true)}>
            <SettingsIcon size={16} />
          </button>
          <div className="clock">
            <strong>{clock.toLocaleTimeString([], { hour12: false })}</strong>
            <span>{message}</span>
          </div>
        </div>
      </header>

      <section className="metrics" aria-label="Session metrics">
        <Metric className="attn" label="attention" value={stats.attention} icon={<BellRing size={15} />} />
        <Metric className="wait" label="waiting" value={stats.waiting} icon={<CircleDot size={15} />} />
        <Metric className="blocked" label="blocked" value={stats.blocked} icon={<ShieldAlert size={15} />} />
        <Metric className="drift" label="drift" value={stats.drift} icon={<Radar size={15} />} />
        <Metric className="work" label="working" value={stats.working} icon={<SatelliteDish size={15} />} />
        <Metric className="idle" label="idle" value={stats.idle} icon={<RotateCcw size={15} />} />
        <Metric className="bound" label="bound" value={stats.bound} icon={<PanelRightOpen size={15} />} />
        <Metric className="hidden" label="hidden" value={stats.hidden} icon={<EyeOff size={15} />} />
      </section>

      <section className="cockpit-band">
        <MasterCaution tasks={tasks} onSelectTask={selectTaskID} />
        <MissionRadar missions={snapshot?.missions ?? []} selectedTaskID={selected?.id ?? ""} onSelectTask={selectTaskID} />
      </section>

      <section className="main-grid" style={{ "--board-width": `${splitPercent}%` } as React.CSSProperties}>
        <section className="board">
          <div className="section-head">
            <strong>mission board</strong>
            <span>{boardMode === "missions" ? `${snapshot?.missions?.length ?? 0} mission groups` : "local wezterm / claude / codex"}</span>
          </div>

          {boardMode === "missions" ? (
            <MissionBoard
              missions={snapshot?.missions ?? []}
              tasks={tasks}
              selectedID={selected?.id ?? ""}
              readonly={readonly}
              onSelectTask={selectTaskID}
              onRenameMission={(missionID, name) => void handleRenameMission(missionID, name)}
            />
          ) : lanes.map((lane) => {
            const laneTasks = tasks.filter((task) => task.status === lane.status);
            if (laneTasks.length === 0) {
              return null;
            }
            return (
              <div className="lane" key={lane.status}>
                <div className="lane-title">
                  <span>{lane.title} / {laneTasks.length}</span>
                </div>
                {laneTasks.map((task) => (
                  <div
                    role="button"
                    tabIndex={0}
                    title={readonly ? "Readonly LAN view: select task" : task.binding.pane ? "Double-click or press Enter to open the bound WezTerm pane" : "Select task"}
                    className={`task-row flight-strip ${task.id === selected?.id ? "selected" : ""} ${launchingID === task.id ? "launching" : ""} ${isHidden(task) ? "hidden-row" : ""} ${hasInputAlert(task) ? "input-alert" : ""}`}
                    key={task.id}
                    onClick={() => setSelectedID(task.id)}
                    onDoubleClick={() => {
                      if (!readonly) {
                        void handleOpen(task);
                      }
                    }}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" && !readonly) {
                        event.preventDefault();
                        void handleOpen(task);
                      }
                    }}
                  >
                    <span className={`status-pill ${statusClass(task.status)} ${hasInputAlert(task) ? "sound-alert" : ""}`}>
                      {hasInputAlert(task) ? <BellRing size={12} /> : null}
                      {statusLabel[task.status]}
                    </span>
                    <span className="agent">{task.session.agent}</span>
                    <span className="project callsign">{callsign(task)}</span>
                    <span className="age">{relativeAge(task.session.last_event_at)}</span>
                    <span
                      className={`link ${manualBinding(task) ? "manual" : task.binding.pane ? "bound" : ""} ${readonly ? "readonly" : ""}`}
                      onClick={(event) => {
                        if (!task.binding.pane || readonly) {
                          return;
                        }
                        event.stopPropagation();
                        setSelectedID(task.id);
                        void handleOpen(task);
                      }}
                    >
                      {task.binding.pane ? <MonitorUp size={12} /> : null}
                      <span>{bindingLabel(task)}</span>
                    </span>
                    <DiffHeatStrip diff={task.diff} />
                    <span className="summary">
                      {hasInputAlert(task) ? <span className="input-alert-chip">RANG {relativeAge(task.input_alert_at)}</span> : null}
                      {task.summary || task.attention_reason || "no summary yet"}
                    </span>
                  </div>
                ))}
              </div>
            );
          })}

          {tasks.length === 0 && (
            <div className="empty-state">
              <SatelliteDish size={22} />
              <span>No active Claude/Codex sessions found.</span>
            </div>
          )}
        </section>

        <div className="splitter" role="separator" aria-label="Resize mission board" onPointerDown={startSplitResize} />

        <aside className={`detail ${launchingID === selected?.id ? "launching" : ""}`}>
          <div className="section-head">
            <strong>selected task</strong>
            <span>{selected ? bindingLabel(selected) : "none"}</span>
          </div>
          <TaskDetail
            task={selected}
            panes={snapshot?.panes ?? []}
            refreshing={refreshingID === selected?.id}
            debriefing={debriefingID === selected?.id}
            summaryEnabled={settings.enable_llm_summary}
            readonly={readonly}
            expandedPanel={visiblePanel}
            closingPanel={closingPanel}
            onExpandPanel={openPanel}
            onClosePanel={closePanel}
            onOpen={() => void handleOpen(selected)}
            onSummary={() => void handleSummary(selected)}
            onDebrief={() => void handleDebrief(selected)}
            onArchive={() => void handleArchive(selected)}
            onIgnore={() => void handleIgnore(selected)}
            onRestore={() => void handleRestore(selected)}
            onAttach={(paneID) => void handleAttach(selected, paneID)}
            onDetach={() => void handleDetach(selected)}
          />
        </aside>
      </section>

      <footer className="footer">
        <span>refreshed {snapshot ? timeOnly(snapshot.built_at) : "--:--:--"}</span>
        <span>{snapshot?.errors?.length ? snapshot.errors.join(" / ") : "systems nominal"}</span>
      </footer>
      </main>
      <button
        className={`panel-overlay ${visiblePanel ? "visible" : ""}`}
        type="button"
        aria-label="Restore expanded panel"
        onClick={closePanel}
      />
      {settingsOpen ? (
        <SettingsPanel settings={settings} saving={savingSettings} onClose={() => setSettingsOpen(false)} onSave={(next) => void handleSaveSettings(next)} />
      ) : null}
      {inboxOpen ? (
        <DoneInboxPanel
          items={doneInbox}
          readonly={readonly}
          onClose={() => setInboxOpen(false)}
          onOpenItem={(item) => {
            if (readonly) {
              blockReadonlyAction();
              return;
            }
            selectTaskID(item.task_id);
            setInboxOpen(false);
            const task = tasks.find((candidate) => candidate.id === item.task_id);
            if (task) {
              void handleOpen(task);
            } else if (item.pane_id !== undefined && !readonly) {
              void openPane(item.pane_id);
            }
          }}
          onReview={(item) => void handleReviewDone(item)}
          onArchive={(item) => void handleArchiveDone(item)}
        />
      ) : null}
      {commandOpen ? (
        <CommandPalette
          tasks={tasks}
          missions={snapshot?.missions ?? []}
          readonly={readonly}
          onClose={() => setCommandOpen(false)}
          onSelectTask={(taskID) => {
            selectTaskID(taskID);
            setCommandOpen(false);
          }}
          onOpenTask={(task) => {
            setSelectedID(task.id);
            setCommandOpen(false);
            void handleOpen(task);
          }}
          onDebrief={(task) => {
            setSelectedID(task.id);
            setCommandOpen(false);
            void handleDebrief(task);
          }}
        />
      ) : null}
      {promptSearchOpen ? (
        <PromptSearchPanel
          onClose={() => setPromptSearchOpen(false)}
          onSelectTask={(taskID) => {
            selectTaskID(taskID);
            setPromptSearchOpen(false);
          }}
        />
      ) : null}
      {ambientOpen ? (
        <AmbientMode
          tasks={tasks}
          missions={snapshot?.missions ?? []}
          stats={stats}
          clock={clock}
          nativeRuntime={nativeRuntime}
          onClose={() => setAmbientOpen(false)}
          onSelectTask={(taskID) => {
            selectTaskID(taskID);
            setAmbientOpen(false);
          }}
        />
      ) : null}
      {webWelcomeOpen ? <WebWelcomeModal owner={cockpitOwner} readonly={readonly} source={source} onClose={dismissWebWelcome} /> : null}
    </>
  );
}

function WebWelcomeModal({ owner, readonly, source, onClose }: { owner: string; readonly: boolean; source: string; onClose: () => void }) {
  const ownerLabel = owner ? `这是 ${owner} 的驾驶舱` : readonly ? "Readonly flight deck" : "Browser preview";
  return (
    <div className="settings-overlay welcome-overlay" role="dialog" aria-modal="true" aria-label="Welcome to Cockpit">
      <div className="welcome-dialog">
        <div className="welcome-beacon" aria-hidden="true">
          <SatelliteDish size={28} />
          <span />
        </div>
        <div className="welcome-copy">
          <span className="welcome-kicker">COCKPIT / {source}</span>
          <h2>Welcome aboard</h2>
          <p>
            <strong>{ownerLabel}</strong>
            <span>{readonly ? "Live LAN view. Watch only." : "Browser preview. Mock controls only."}</span>
          </p>
        </div>
        <div className="welcome-status-grid">
          <div>
            <span>mode</span>
            <strong>{readonly ? "watch only" : "preview"}</strong>
          </div>
          <div>
            <span>refresh</span>
            <strong>5s sweep</strong>
          </div>
          <div>
            <span>control</span>
            <strong>{readonly ? "local app" : "mock data"}</strong>
          </div>
        </div>
        <button className="action-button primary welcome-action" type="button" onClick={onClose}>
          Enter Cockpit
        </button>
      </div>
    </div>
  );
}

function Metric({
  label,
  value,
  className,
  icon,
}: {
  label: string;
  value: number;
  className: string;
  icon: React.ReactNode;
}) {
  return (
    <div className={`metric ${className}`}>
      <div className="metric-label">
        {icon}
        <span>{label}</span>
      </div>
      <div className="metric-value">{String(value).padStart(2, "0")}</div>
    </div>
  );
}

function BrandGauge() {
  const needleRef = useRef<SVGGElement | null>(null);

  useEffect(() => {
    let frame = 0;
    let phaseStartedAt = performance.now();
    let phase: "sweep-up" | "hold-top" | "sweep-down" | "hold-zero" = "sweep-up";
    const sweepUpMs = 760;
    const holdTopMs = 500;
    const sweepDownMs = 680;
    const holdZeroMs = 5000;
    const nextTargetAngle = () => 60 + Math.random() * 110;
    let targetAngle = nextTargetAngle();
    const ease = (value: number) => 0.5 - Math.cos(Math.min(1, Math.max(0, value)) * Math.PI) / 2;
    const setNeedle = (angle: number) => {
      needleRef.current?.setAttribute("transform", `rotate(${angle.toFixed(2)} 18 22)`);
    };
    const tick = (now: number) => {
      const elapsed = now - phaseStartedAt;
      if (phase === "sweep-up") {
        setNeedle(ease(elapsed / sweepUpMs) * targetAngle);
        if (elapsed >= sweepUpMs) {
          phase = "hold-top";
          phaseStartedAt = now;
          setNeedle(targetAngle);
        }
      } else if (phase === "hold-top") {
        setNeedle(targetAngle);
        if (elapsed >= holdTopMs) {
          phase = "sweep-down";
          phaseStartedAt = now;
        }
      } else if (phase === "sweep-down") {
        setNeedle((1 - ease(elapsed / sweepDownMs)) * targetAngle);
        if (elapsed >= sweepDownMs) {
          phase = "hold-zero";
          phaseStartedAt = now;
          setNeedle(0);
        }
      } else {
        setNeedle(0);
        if (elapsed >= holdZeroMs) {
          targetAngle = nextTargetAngle();
          phase = "sweep-up";
          phaseStartedAt = now;
        }
      }
      frame = window.requestAnimationFrame(tick);
    };
    frame = window.requestAnimationFrame(tick);
    return () => window.cancelAnimationFrame(frame);
  }, []);

  return (
    <svg className="brand-gauge" viewBox="0 0 36 36" role="img" aria-label="Cockpit gauge">
      <path className="brand-gauge-arc dim" d="M8 22a10 10 0 0 1 20 0" />
      <path className="brand-gauge-arc live" d="M8 22a10 10 0 0 1 20 0" />
      <g className="brand-gauge-needle" ref={needleRef}>
        <line x1="18" y1="22" x2="8" y2="22" />
      </g>
      <circle className="brand-gauge-hub" cx="18" cy="22" r="2.2" />
      <path className="brand-gauge-tick" d="M10.8 20.7h2.4M22.8 12.8l-1.2 2.1M25.2 20.7h-2.4" />
    </svg>
  );
}

function MasterCaution({ tasks, onSelectTask }: { tasks: Task[]; onSelectTask: (taskID: string) => void }) {
  const cautions = tasks
    .filter((task) => ["needs_attention", "blocked", "drift", "waiting"].includes(task.status))
    .sort((a, b) => Number(hasInputAlert(b)) - Number(hasInputAlert(a)) || statusSeverity(b.status) - statusSeverity(a.status))
    .slice(0, 3);
  if (cautions.length === 0) {
    return null;
  }
  return (
    <section className="master-caution has-alerts" aria-label="Master caution">
      <div className="caution-title">
        <BellRing size={14} />
        <strong>MASTER CAUTION</strong>
      </div>
      <div className="caution-stack">
        {cautions.map((task) => (
          <button className={`caution-chip ${statusClass(task.status)} ${hasInputAlert(task) ? "input-alert" : ""}`} type="button" key={task.id} onClick={() => onSelectTask(task.id)}>
            <span>{hasInputAlert(task) ? "RANG" : statusLabel[task.status]}</span>
            <strong>{repoName(task)}</strong>
            <em>{task.input_alert_reason || task.attention_reason || task.status_explanation?.reason || task.summary || "attention required"}</em>
          </button>
        ))}
      </div>
    </section>
  );
}

function MissionRadar({
  missions,
  selectedTaskID,
  onSelectTask,
}: {
  missions: Mission[];
  selectedTaskID: string;
  onSelectTask: (taskID: string) => void;
}) {
  const plotted = missions.slice(0, 8);
  const selectedMission = missions.find((mission) => mission.task_ids.includes(selectedTaskID)) ?? plotted[0];
  const selectedIndex = Math.max(0, plotted.findIndex((mission) => mission.id === selectedMission?.id));
  const selectedPoint = selectedMission ? radarPoint(selectedIndex, plotted.length) : undefined;
  const targetList = missions.slice(0, 6);
  return (
    <section className="mission-radar" aria-label="Mission radar">
      <div className="radar-scope">
        <div className="radar-grid" />
        <svg className="radar-vector" viewBox="0 0 100 100" aria-hidden="true">
          <path d="M12 66 C28 54, 38 64, 50 49 S73 32, 88 42" />
          <path d="M20 28 C34 36, 43 24, 57 31 S78 50, 84 62" />
        </svg>
        <div className="radar-sweep" />
        <div className="radar-ring one" />
        <div className="radar-ring two" />
        <div className="radar-ring three" />
        <span className="radar-bearing north">N</span>
        <span className="radar-bearing east">E</span>
        <span className="radar-bearing south">S</span>
        <span className="radar-bearing west">W</span>
        <span className="radar-hud-label range">RANGE {Math.max(6, missions.length * 3)}KM</span>
        {selectedMission ? <span className="radar-hud-label lock">LOCKED</span> : null}
        <div className="radar-core" />
        {selectedPoint ? <div className="radar-lock" style={{ left: `${selectedPoint.x}%`, top: `${selectedPoint.y}%` }} /> : null}
        {plotted.map((mission, index) => {
          const point = radarPoint(index, plotted.length);
          const selected = mission.task_ids.includes(selectedTaskID);
          return (
            <button
              className={`radar-dot ${statusClass(mission.status)} ${selected ? "selected" : ""}`}
              style={{ left: `${point.x}%`, top: `${point.y}%` }}
              type="button"
              title={`${mission.name} / ${mission.status}`}
              key={mission.id}
              onClick={() => onSelectTask(mission.task_ids[0] ?? "")}
            >
              <span className="blip-trail" />
              <span className="blip-label">{missionCallsign(mission)}</span>
            </button>
          );
        })}
      </div>
      <div className="radar-list">
        <div className="radar-copy">
          <strong>MISSION RADAR</strong>
          <span>
            {missions.length} groups tracked
            {selectedMission ? ` / selected ${missionCallsign(selectedMission)} / signal locked` : " / scanning"}
          </span>
        </div>
        <div className="radar-telemetry">
          <div><span>scan</span><strong>5.2s sweep</strong></div>
          <div><span>signal</span><strong>{selectedMission ? `${selectedMission.task_ids.length} task lock` : "no lock"}</strong></div>
          <div><span>risk</span><strong>{selectedMission ? `${statusLabel[selectedMission.status]} / ${selectedMission.changed_files} files` : "clear"}</strong></div>
          <div><span>mode</span><strong>live watch</strong></div>
        </div>
        <div className="radar-targets">
          {targetList.length === 0 ? (
            <span className="radar-empty">no mission groups yet</span>
          ) : targetList.map((mission) => (
            <button className={`radar-target ${statusClass(mission.status)}`} type="button" key={mission.id} onClick={() => onSelectTask(mission.task_ids[0] ?? "")}>
              <i />
              <strong>{missionCallsign(mission)}</strong>
              <span>{statusLabel[mission.status]}</span>
              <em>{mission.summary || mission.cwd || mission.name}</em>
            </button>
          ))}
        </div>
      </div>
    </section>
  );
}

function DiffHeatStrip({ diff }: { diff?: DiffRadar }) {
  const files = diff?.files ?? [];
  const cells = files.length > 0 ? files.slice(0, 8) : [];
  if (!diff?.dirty || cells.length === 0) {
    return <span className="diff-heat empty" title="No file changes" />;
  }
  return (
    <span className="diff-heat" title={diff.summary || "changed files"}>
      {cells.map((file, index) => (
        <i className={diffHeatClass(file.path, file.status)} key={`${file.path}-${index}`} />
      ))}
    </span>
  );
}

function MissionBoard({
  missions,
  tasks,
  selectedID,
  readonly,
  onSelectTask,
  onRenameMission,
}: {
  missions: Mission[];
  tasks: Task[];
  selectedID: string;
  readonly: boolean;
  onSelectTask: (taskID: string) => void;
  onRenameMission: (missionID: string, name: string) => void;
}) {
  const [editingID, setEditingID] = useState("");
  const [draftName, setDraftName] = useState("");
  if (missions.length === 0) {
    return (
      <div className="empty-state">
        <Radar size={22} />
        <span>No mission groups yet.</span>
      </div>
    );
  }
  const taskByID = new Map(tasks.map((task) => [task.id, task]));
  return (
    <div className="mission-list">
      {missions.map((mission) => {
        const missionTasks = mission.task_ids.map((id) => taskByID.get(id)).filter((task): task is Task => Boolean(task));
        const selected = mission.task_ids.includes(selectedID);
        const editing = editingID === mission.id;
        return (
          <div className={`mission-card ${selected ? "selected" : ""}`} key={mission.id}>
            <div className="mission-main-row">
              <button className="mission-main" type="button" onClick={() => onSelectTask(mission.task_ids[0] ?? "")}>
                <span className={`status-pill ${statusClass(mission.status)}`}>{statusLabel[mission.status]}</span>
                <div>
                  <strong>{mission.name}</strong>
                  <span>{mission.summary || mission.cwd || "mission group"}</span>
                </div>
                <em>{mission.changed_files} files</em>
              </button>
              <button
                className="mission-rename-button"
                type="button"
                title={readonly ? "Readonly LAN view" : "Rename mission"}
                disabled={readonly}
                onClick={() => {
                  setEditingID(mission.id);
                  setDraftName(mission.renamed ? mission.name : "");
                }}
              >
                <Pencil size={13} />
              </button>
            </div>
            {editing ? (
              <form
                className="mission-rename-form"
                onSubmit={(event) => {
                  event.preventDefault();
                  onRenameMission(mission.id, draftName);
                  setEditingID("");
                }}
              >
                <input
                  autoFocus
                  value={draftName}
                  placeholder={mission.default_name || mission.name}
                  onChange={(event) => setDraftName(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Escape") {
                      event.preventDefault();
                      setEditingID("");
                    }
                  }}
                />
                <button type="submit" title="Save mission name">
                  <Check size={13} />
                </button>
                <button
                  type="button"
                  title="Reset to detected name"
                  onClick={() => {
                    onRenameMission(mission.id, "");
                    setEditingID("");
                  }}
                >
                  <RotateCcw size={13} />
                </button>
                <button type="button" title="Cancel rename" onClick={() => setEditingID("")}>
                  <X size={13} />
                </button>
              </form>
            ) : null}
            <div className="mission-stats">
              <span>tasks {mission.task_ids.length}</span>
              <span>attn {mission.attention}</span>
              <span>blocked {mission.blocked}</span>
              <span>drift {mission.drift}</span>
              <span>work {mission.working}</span>
              <span>done {mission.completed}</span>
            </div>
            <div className="mission-children">
              {missionTasks.slice(0, 5).map((task) => (
                <button type="button" className={task.id === selectedID ? "active" : ""} key={task.id} onClick={() => onSelectTask(task.id)}>
                  <span className={`status-pill ${statusClass(task.status)}`}>{statusLabel[task.status]}</span>
                  <span>{task.session.title || repoName(task)}</span>
                </button>
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function DiffRadarPanel({
  diff,
  expanded,
  closing,
  onToggle,
}: {
  diff?: DiffRadar;
  expanded: boolean;
  closing: boolean;
  onToggle: () => void;
}) {
  const clean = !diff || (!diff.dirty && !diff.last_error);
  const files = diff?.files ?? [];
  const activeDiff = diff;
  return (
    <div className={`diff-radar-panel instrument-panel ${clean ? "clean" : "dirty"} ${expanded ? "is-expanded" : ""} ${closing ? "is-closing" : ""}`}>
      <div className="diff-head panel-head">
        <div className="panel-title">
          <strong>diff radar</strong>
          <span>{diff?.branch || diff?.repo_root || diff?.last_error || "working tree"}</span>
        </div>
        <button
          className="icon-button panel-toggle"
          type="button"
          title={expanded ? "Restore diff radar" : "Expand diff radar"}
          aria-label={expanded ? "Restore diff radar" : "Expand diff radar"}
          aria-expanded={expanded}
          onClick={onToggle}
        >
          {expanded ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
        </button>
      </div>
      <div className="panel-body diff-panel-body">
        {clean ? (
          <div className="diff-empty">
            <GitBranch size={16} />
            <span>{diff?.last_error || "no working tree changes detected"}</span>
          </div>
        ) : activeDiff ? (
          <>
            <div className="diff-stat-grid">
              <div><span>modified</span><strong>{activeDiff.modified}</strong></div>
              <div><span>untracked</span><strong>{activeDiff.untracked}</strong></div>
              <div><span>tests</span><strong>{activeDiff.tests}</strong></div>
              <div><span>locks</span><strong>{activeDiff.lockfiles}</strong></div>
            </div>
            <div className="diff-summary">
              <span>{activeDiff.summary || activeDiff.last_error || "working tree state unavailable"}</span>
              <span>{files.length} files</span>
              <span>{activeDiff.added} added</span>
              <span>{activeDiff.deleted} deleted</span>
            </div>
            {files.length > 0 ? (
              <div className="diff-files">
                {files.slice(0, expanded ? 40 : 12).map((file) => (
                  <span className={`diff-file ${file.status}`} key={`${file.status}-${file.path}`}>
                    <em>{file.status}</em>
                    <strong>{file.path}</strong>
                    {file.additions || file.deletions ? <small>+{file.additions ?? 0} -{file.deletions ?? 0}</small> : <small />}
                  </span>
                ))}
              </div>
            ) : null}
          </>
        ) : null}
      </div>
    </div>
  );
}

function DebriefPanel({
  task,
  expanded,
  closing,
  onToggle,
}: {
  task: Task;
  expanded: boolean;
  closing: boolean;
  onToggle: () => void;
}) {
  const hasDebrief = Boolean(task.debrief?.text);
  return (
    <div className={`debrief-panel instrument-panel ${expanded ? "is-expanded" : ""} ${closing ? "is-closing" : ""}`}>
      <div className="debrief-head panel-head">
        <div className="panel-title">
          <strong>debrief</strong>
          <span>{hasDebrief ? timeOnly(task.debrief?.generated_at) : "not generated"}</span>
        </div>
        <button
          className="icon-button panel-toggle"
          type="button"
          title={expanded ? "Restore debrief" : "Expand debrief"}
          aria-label={expanded ? "Restore debrief" : "Expand debrief"}
          aria-expanded={expanded}
          onClick={onToggle}
        >
          {expanded ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
        </button>
      </div>
      <div className="panel-body debrief-body">
        {task.debrief?.text ? (
          <MarkdownView text={task.debrief.text} />
        ) : (
          <div className="empty-state compact">
            <FileText size={16} />
            <span>No debrief generated yet.</span>
          </div>
        )}
      </div>
    </div>
  );
}

type MarkdownBlock =
  | { kind: "heading"; level: number; text: string }
  | { kind: "list"; ordered: boolean; items: string[] }
  | { kind: "code"; text: string }
  | { kind: "paragraph"; text: string };

function MarkdownView({ text }: { text: string }) {
  const blocks = markdownBlocks(text);
  return (
    <div className="markdown-view">
      {blocks.map((block, index) => {
        switch (block.kind) {
          case "heading": {
            const level = Math.min(Math.max(block.level, 2), 4);
            const Tag = `h${level}` as "h2" | "h3" | "h4";
            return <Tag key={`${block.kind}-${index}`}>{renderInlineMarkdown(block.text)}</Tag>;
          }
          case "list": {
            const Tag = block.ordered ? "ol" : "ul";
            return (
              <Tag key={`${block.kind}-${index}`}>
                {block.items.map((item, itemIndex) => (
                  <li key={`${item}-${itemIndex}`}>{renderInlineMarkdown(item)}</li>
                ))}
              </Tag>
            );
          }
          case "code":
            return <pre key={`${block.kind}-${index}`}><code>{block.text}</code></pre>;
          default:
            return <p key={`${block.kind}-${index}`}>{renderInlineMarkdown(block.text)}</p>;
        }
      })}
    </div>
  );
}

function markdownBlocks(text: string): MarkdownBlock[] {
  const lines = text.replace(/\r\n/g, "\n").split("\n");
  const blocks: MarkdownBlock[] = [];
  let paragraph: string[] = [];
  let list: { ordered: boolean; items: string[] } | null = null;
  let code: string[] | null = null;

  const flushParagraph = () => {
    if (paragraph.length === 0) {
      return;
    }
    blocks.push({ kind: "paragraph", text: paragraph.join(" ") });
    paragraph = [];
  };
  const flushList = () => {
    if (!list) {
      return;
    }
    blocks.push({ kind: "list", ordered: list.ordered, items: list.items });
    list = null;
  };

  for (const rawLine of lines) {
    const line = rawLine.trimEnd();
    if (line.trim().startsWith("```")) {
      flushParagraph();
      flushList();
      if (code) {
        blocks.push({ kind: "code", text: code.join("\n") });
        code = null;
      } else {
        code = [];
      }
      continue;
    }
    if (code) {
      code.push(line);
      continue;
    }
    if (line.trim() === "") {
      flushParagraph();
      flushList();
      continue;
    }
    const heading = /^(#{1,4})\s+(.+)$/.exec(line);
    if (heading) {
      flushParagraph();
      flushList();
      blocks.push({ kind: "heading", level: heading[1].length, text: heading[2].trim() });
      continue;
    }
    const unordered = /^\s*[-*]\s+(.+)$/.exec(line);
    const ordered = /^\s*\d+\.\s+(.+)$/.exec(line);
    if (unordered || ordered) {
      flushParagraph();
      const item = (unordered?.[1] ?? ordered?.[1] ?? "").trim();
      const isOrdered = Boolean(ordered);
      if (!list || list.ordered !== isOrdered) {
        flushList();
        list = { ordered: isOrdered, items: [] };
      }
      list.items.push(item);
      continue;
    }
    flushList();
    paragraph.push(line.trim());
  }
  flushParagraph();
  flushList();
  if (code) {
    blocks.push({ kind: "code", text: code.join("\n") });
  }
  return blocks;
}

function renderInlineMarkdown(text: string) {
  return text.split(/(`[^`]+`|\*\*[^*]+\*\*)/g).filter(Boolean).map((part, index) => {
    if (part.startsWith("`") && part.endsWith("`")) {
      return <code key={`${part}-${index}`}>{part.slice(1, -1)}</code>;
    }
    if (part.startsWith("**") && part.endsWith("**")) {
      return <strong key={`${part}-${index}`}>{part.slice(2, -2)}</strong>;
    }
    return <span key={`${part}-${index}`}>{part}</span>;
  });
}

function FlightRecorder({
  events,
  expanded,
  closing,
  onToggle,
}: {
  events: FlightEvent[];
  expanded: boolean;
  closing: boolean;
  onToggle: () => void;
}) {
  const sortedDesc = [...events].sort((a, b) => timeValue(b.at) - timeValue(a.at));
  const sortedAsc = [...events].sort((a, b) => timeValue(a.at) - timeValue(b.at));
  const [replayIndex, setReplayIndex] = useState(Math.max(0, sortedAsc.length - 1));
  useEffect(() => {
    setReplayIndex(Math.max(0, sortedAsc.length - 1));
  }, [events.length]);
  const activeReplay = sortedAsc[Math.min(replayIndex, Math.max(0, sortedAsc.length - 1))];
  const visible = expanded ? sortedDesc : sortedDesc.slice(0, 12);
  return (
    <div className={`flight-recorder instrument-panel ${expanded ? "is-expanded" : ""} ${closing ? "is-closing" : ""}`}>
      <div className="flight-head panel-head">
        <div className="panel-title">
          <strong>flight recorder</strong>
          <span>{expanded ? `${events.length} events` : `${visible.length}/${events.length} events`}</span>
        </div>
        <button
          className="icon-button panel-toggle"
          type="button"
          title={expanded ? "Restore flight recorder" : "Expand flight recorder"}
          aria-label={expanded ? "Restore flight recorder" : "Expand flight recorder"}
          aria-expanded={expanded}
          onClick={onToggle}
        >
          {expanded ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
        </button>
      </div>
      {expanded && sortedAsc.length > 0 ? (
        <div className="replay-console">
          <div>
            <strong>{activeReplay ? timeOnly(activeReplay.at) : "--:--:--"}</strong>
            <span>{activeReplay ? `${activeReplay.kind} / ${activeReplay.title}` : "no replay event"}</span>
          </div>
          <input
            type="range"
            min={0}
            max={Math.max(0, sortedAsc.length - 1)}
            value={Math.min(replayIndex, Math.max(0, sortedAsc.length - 1))}
            onChange={(event) => setReplayIndex(Number(event.target.value))}
          />
        </div>
      ) : null}
      <div className="panel-body flight-body">
        {visible.length === 0 ? (
          <div className="event empty">
            <time>--:--:--</time>
            <span className="type">idle</span>
            <span className="text">no flight events recorded</span>
          </div>
        ) : (
          visible.map((event) => (
            <div className={`event flight-${event.kind} ${activeReplay?.id === event.id ? "replay-active" : ""}`} key={event.id}>
              <time>{timeOnly(event.at)}</time>
              <span className="type">{event.kind}</span>
              <span className="text">
                {event.detail ? `${event.title}: ${event.detail}` : event.title}
                {expanded && event.source ? <em>{event.source}</em> : null}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

function DoneInboxPanel({
  items,
  readonly,
  onClose,
  onOpenItem,
  onReview,
  onArchive,
}: {
  items: DoneInboxItem[];
  readonly: boolean;
  onClose: () => void;
  onOpenItem: (item: DoneInboxItem) => void;
  onReview: (item: DoneInboxItem) => void;
  onArchive: (item: DoneInboxItem) => void;
}) {
  return (
    <div className="settings-overlay" role="dialog" aria-modal="true" aria-label="Done inbox">
      <div className="settings-dialog inbox-dialog">
        <div className="settings-head">
          <div>
            <strong>done inbox</strong>
            <span>{items.filter((item) => !item.reviewed).length} new / {items.length} total</span>
          </div>
          <button className="icon-button" type="button" title="Close done inbox" onClick={onClose}>
            <X size={15} />
          </button>
        </div>
        <div className="settings-body inbox-list">
          {items.length === 0 ? (
            <div className="empty-state">
              <ClipboardCheck size={22} />
              <span>No completed tasks waiting for review.</span>
            </div>
          ) : (
            items.map((item) => (
              <div className={`inbox-item ${item.reviewed ? "reviewed" : ""}`} key={item.id}>
                <div>
                  <strong>{item.title || item.project}</strong>
                  <span>{item.agent} / {timeOnly(item.completed_at)} / {item.diff?.summary || "no diff"}</span>
                  <p>{item.summary || "no final summary"}</p>
                </div>
                <div className="inbox-actions">
                  <button className="action-button primary" type="button" disabled={readonly} onClick={() => onOpenItem(item)}>Open</button>
                  <button className="action-button" type="button" disabled={readonly} onClick={() => onReview(item)}>Review</button>
                  <button className="icon-button" type="button" title={readonly ? "Readonly LAN view" : "Archive done item"} disabled={readonly} onClick={() => onArchive(item)}>
                    <Archive size={15} />
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}

function CommandPalette({
  tasks,
  missions,
  readonly,
  onClose,
  onSelectTask,
  onOpenTask,
  onDebrief,
}: {
  tasks: Task[];
  missions: Mission[];
  readonly: boolean;
  onClose: () => void;
  onSelectTask: (taskID: string) => void;
  onOpenTask: (task: Task) => void;
  onDebrief: (task: Task) => void;
}) {
  const [query, setQuery] = useState("");
  const matches = commandMatches(tasks, missions, query);
  const missionMatches = matches.filter((match) => match.kind === "mission").slice(0, 5);
  const taskMatches = matches.filter((match) => match.kind === "task").slice(0, 8);
  const primaryTask = taskMatches[0]?.kind === "task" ? taskMatches[0].task : tasks[0];
  useEffect(() => {
    const handle = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", handle);
    return () => window.removeEventListener("keydown", handle);
  }, [onClose]);
  return (
    <div className="settings-overlay command-overlay" role="dialog" aria-modal="true" aria-label="Command palette">
      <div className="command-dialog">
        <div className="command-input">
          <Search size={16} />
          <input autoFocus value={query} placeholder="Search sessions, missions, cwd, summary..." onChange={(event) => setQuery(event.target.value)} />
          <button className="icon-button" type="button" title="Close command palette" onClick={onClose}>
            <X size={15} />
          </button>
        </div>
        <div className="command-list">
          <div className="command-section">
            <span className="command-section-title">Actions</span>
            <div className="command-action-grid">
              <button type="button" disabled={!primaryTask || readonly} onClick={() => primaryTask && onOpenTask(primaryTask)}>
                <MonitorUp size={14} />
                <strong>Open Pane</strong>
              </button>
              <button type="button" disabled={!primaryTask || readonly} onClick={() => primaryTask && onDebrief(primaryTask)}>
                <FileText size={14} />
                <strong>Generate Debrief</strong>
              </button>
              <button type="button" onClick={() => onClose()}>
                <X size={14} />
                <strong>Close HUD</strong>
              </button>
            </div>
          </div>
          {missionMatches.length > 0 ? (
            <div className="command-section">
              <span className="command-section-title">Missions</span>
              {missionMatches.map((match) =>
                match.kind === "mission" ? (
                  <button className="command-mission" type="button" key={match.id} onClick={() => onSelectTask(match.taskID ?? "")}>
                    <Radar size={15} />
                    <strong>{match.title}</strong>
                    <span>{match.detail}</span>
                  </button>
                ) : null,
              )}
            </div>
          ) : null}
          <div className="command-section">
            <span className="command-section-title">Sessions</span>
            {taskMatches.map((match) => {
              if (match.kind !== "task") {
                return null;
              }
              const task = match.task;
              return (
                <div className="command-result" key={task.id}>
                  <button type="button" onClick={() => onSelectTask(task.id)}>
                    <span className={`status-pill ${statusClass(task.status)}`}>{statusLabel[task.status]}</span>
                    <strong>{task.session.title || repoName(task)}</strong>
                    <span>{task.summary || task.session.cwd}</span>
                  </button>
                  <button type="button" title={readonly ? "Readonly LAN view" : "Open pane"} disabled={readonly} onClick={() => onOpenTask(task)}>
                    <MonitorUp size={14} />
                  </button>
                  <button type="button" title={readonly ? "Readonly LAN view" : "Generate debrief"} disabled={readonly} onClick={() => onDebrief(task)}>
                    <FileText size={14} />
                  </button>
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}

function PromptSearchPanel({
  onClose,
  onSelectTask,
}: {
  onClose: () => void;
  onSelectTask: (taskID: string) => void;
}) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<PromptSearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [copiedID, setCopiedID] = useState("");
  const requestSeq = useRef(0);

  const runSearch = useCallback(async (nextQuery: string) => {
    const seq = requestSeq.current + 1;
    requestSeq.current = seq;
    setLoading(true);
    setError("");
    try {
      const nextResults = await searchPrompts(nextQuery);
      if (requestSeq.current === seq) {
        setResults(Array.isArray(nextResults) ? nextResults : []);
      }
    } catch (searchError) {
      if (requestSeq.current === seq) {
        setError(searchError instanceof Error ? searchError.message : "prompt search failed");
        setResults([]);
      }
    } finally {
      if (requestSeq.current === seq) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void runSearch(query), 320);
    return () => window.clearTimeout(timer);
  }, [query, runSearch]);

  useEffect(() => {
    const handle = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", handle);
    return () => window.removeEventListener("keydown", handle);
  }, [onClose]);

  const copyPrompt = async (result: PromptSearchResult) => {
    try {
      await copyText(result.prompt);
      setCopiedID(result.id);
      window.setTimeout(() => setCopiedID((current) => (current === result.id ? "" : current)), 1300);
    } catch (copyError) {
      const message = copyError instanceof Error ? copyError.message : "copy prompt failed";
      setError(message);
      void reportFrontendError("prompt copy failed: " + message, copyError instanceof Error ? copyError.stack ?? "" : "");
    }
  };

  return (
    <div className="settings-overlay command-overlay" role="dialog" aria-modal="true" aria-label="Prompt search">
      <div className="prompt-search-dialog">
        <div className="prompt-search-head">
          <div>
            <strong>PROMPT SEARCH</strong>
            <span>cross-session user prompts</span>
          </div>
          <button className="icon-button" type="button" title="Close prompt search" onClick={onClose}>
            <X size={15} />
          </button>
        </div>
        <div className="prompt-search-input">
          <Search size={16} />
          <input autoFocus value={query} placeholder="Search past prompts..." onChange={(event) => setQuery(event.target.value)} />
          <span className="prompt-search-engine">FTS5</span>
        </div>
        <div className="prompt-search-meta">
          <span>{loading ? "scanning logs..." : `${results.length} prompts`}</span>
          <span>gojieba segmentation + SQLite FTS5</span>
        </div>
        <div className="prompt-results">
          {error ? <div className="prompt-empty">{error}</div> : null}
          {!error && results.length === 0 && !loading ? <div className="prompt-empty">No matching prompts yet.</div> : null}
          {results.map((result) => (
            <div className="prompt-result" key={result.id}>
              <button className="prompt-copy-target" type="button" title="Copy this prompt" onClick={() => void copyPrompt(result)}>
                <span className={`agent-dot ${result.agent}`}>{result.agent}</span>
                <strong>{result.prompt}</strong>
                <em>{result.summary || result.cwd || "previous user prompt"}</em>
                <span>{result.mission_name || repoNameFromPath(result.cwd) || result.session_id.slice(0, 8)}</span>
              </button>
              <button className="prompt-copy-button" type="button" title="Copy prompt" onClick={() => void copyPrompt(result)}>
                {copiedID === result.id ? <ClipboardCheck size={14} /> : <Copy size={14} />}
              </button>
              <button className="prompt-focus-button" type="button" title="Select source session" onClick={() => onSelectTask(result.task_id)}>
                <Radar size={14} />
              </button>
              <span className="prompt-result-time">{result.at ? relativeAge(result.at) : "unknown"}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function AmbientMode({
  tasks,
  missions,
  stats,
  clock,
  nativeRuntime,
  onClose,
  onSelectTask,
}: {
  tasks: Task[];
  missions: Mission[];
  stats: ReturnType<typeof collectStats>;
  clock: Date;
  nativeRuntime: boolean;
  onClose: () => void;
  onSelectTask: (taskID: string) => void;
}) {
  const active = tasks.filter((task) => ["needs_attention", "blocked", "drift", "working", "waiting"].includes(task.status)).slice(0, 6);
  return (
    <div className={`ambient-overlay ${nativeRuntime ? "native-ambient" : ""}`} role="dialog" aria-modal="true" aria-label="Ambient cockpit mode">
      <div className="ambient-top">
        <div>
          <strong>COCKPIT AMBIENT</strong>
          <span>{clock.toLocaleTimeString([], { hour12: false })}</span>
        </div>
        <button className="icon-button" type="button" title="Close ambient mode" onClick={onClose}>
          <X size={15} />
        </button>
      </div>
      <div className="ambient-grid">
        <div className="ambient-radar">
          <MissionRadar missions={missions} selectedTaskID="" onSelectTask={onSelectTask} />
        </div>
        <div className="ambient-bottom">
          <div className="ambient-counts">
            <Metric className="attn" label="attention" value={stats.attention} icon={<BellRing size={15} />} />
            <Metric className="blocked" label="blocked" value={stats.blocked} icon={<ShieldAlert size={15} />} />
            <Metric className="drift" label="drift" value={stats.drift} icon={<Radar size={15} />} />
            <Metric className="work" label="working" value={stats.working} icon={<SatelliteDish size={15} />} />
          </div>
          <div className="ambient-feed">
            <strong>ACTIVE FLIGHT STRIPS</strong>
            {active.length === 0 ? (
              <span className="caution-clear">NO ACTIVE SESSIONS</span>
            ) : (
              active.map((task) => (
                <button type="button" key={task.id} onClick={() => onSelectTask(task.id)}>
                  <span className={`status-pill ${statusClass(task.status)}`}>{statusLabel[task.status]}</span>
                  <strong>{callsign(task)}</strong>
                  <em>{task.summary || task.attention_reason || task.session.cwd}</em>
                  <DiffHeatStrip diff={task.diff} />
                </button>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function TaskDetail({
  task,
  panes,
  refreshing,
  debriefing,
  summaryEnabled,
  readonly,
  expandedPanel,
  closingPanel,
  onExpandPanel,
  onClosePanel,
  onOpen,
  onSummary,
  onDebrief,
  onArchive,
  onIgnore,
  onRestore,
  onAttach,
  onDetach,
}: {
  task?: Task;
  panes: Pane[];
  refreshing: boolean;
  debriefing: boolean;
  summaryEnabled: boolean;
  readonly: boolean;
  expandedPanel: ExpandablePanel | null;
  closingPanel: ExpandablePanel | null;
  onExpandPanel: (panel: ExpandablePanel) => void;
  onClosePanel: () => void;
  onOpen: () => void;
  onSummary: () => void;
  onDebrief: () => void;
  onArchive: () => void;
  onIgnore: () => void;
  onRestore: () => void;
  onAttach: (paneID: number) => void;
  onDetach: () => void;
}) {
  const [paneID, setPaneID] = useState("");
  const [activeInspector, setActiveInspector] = useState<InspectorTab>("diff");

  useEffect(() => {
    const current = task?.binding.pane?.pane_id;
    if (current !== undefined) {
      setPaneID(String(current));
      return;
    }
    if (panes.length > 0) {
      setPaneID(String(panes[0].pane_id));
      return;
    }
    setPaneID("");
  }, [panes, task?.binding.pane?.pane_id, task?.id]);

  useEffect(() => {
    setActiveInspector("diff");
  }, [task?.id]);

  if (!task) {
    return (
      <div className="detail-empty">
        <MonitorUp size={24} />
        <span>No task selected.</span>
      </div>
    );
  }

  const events = [...(task.session.events ?? [])].slice(-8).reverse();
  const latest = task.attention_reason || task.status_explanation?.reason || task.summary || "no summary yet";
  return (
    <>
      <div className="selected-card">
        <div className="selected-title">
          <div>
            <h2>{task.session.title || repoName(task)}</h2>
            <span>{task.session.agent} / {shortID(task)}</span>
          </div>
          <span className={`status-pill ${statusClass(task.status)} ${hasInputAlert(task) ? "sound-alert" : ""}`}>
            {hasInputAlert(task) ? <BellRing size={12} /> : null}
            {statusLabel[task.status]}
          </span>
        </div>

        {hasInputAlert(task) ? (
          <div className="callout input-alert-callout">
            <BellRing size={14} />
            <strong>input sound</strong>
            <span>{task.input_alert_reason || "waiting for user input"} / {relativeAge(task.input_alert_at)}</span>
          </div>
        ) : null}

        {(task.attention_reason || task.status === "needs_attention") && (
          <div className="callout">{task.attention_reason || task.summary}</div>
        )}

        <div className="action-row">
          <button className="action-button primary" type="button" title={readonly ? "Readonly LAN view" : "Open pane"} disabled={readonly} onClick={onOpen}>
            <MonitorUp size={15} />
            <span>Open</span>
          </button>
          <button className="action-button" type="button" title={readonly ? "Readonly LAN view" : summaryEnabled ? "Refresh summary" : "LLM summary disabled"} disabled={readonly || !summaryEnabled} onClick={onSummary}>
            <RefreshCw size={15} className={refreshing ? "spin" : ""} />
            <span>Summary</span>
          </button>
          <button className="action-button" type="button" title={readonly ? "Readonly LAN view" : summaryEnabled ? "Generate debrief" : "LLM summary disabled"} disabled={readonly || !summaryEnabled} onClick={onDebrief}>
            <FileText size={15} className={debriefing ? "pulse-icon" : ""} />
            <span>Debrief</span>
          </button>
          <button className="icon-button" type="button" title={readonly ? "Readonly LAN view" : "Archive"} disabled={readonly} onClick={onArchive}>
            <Archive size={15} />
          </button>
          <button className="icon-button" type="button" title={readonly ? "Readonly LAN view" : "Ignore"} disabled={readonly} onClick={onIgnore}>
            <EyeOff size={15} />
          </button>
          {isHidden(task) ? (
            <button className="action-button" type="button" title={readonly ? "Readonly LAN view" : "Restore"} disabled={readonly} onClick={onRestore}>
              <Eye size={15} />
              <span>Restore</span>
            </button>
          ) : null}
        </div>

        <div className="selected-summary-grid">
          <div>
            <span>status</span>
            <strong className={`status-pill ${statusClass(task.status)}`}>{statusLabel[task.status]} {task.status_explanation?.rule || ""}</strong>
          </div>
          <div>
            <span>binding</span>
            <strong>{bindingLabel(task)} / {confidenceLabel(task)}</strong>
          </div>
          <div>
            <span>diff</span>
            <strong>{diffSummaryLabel(task.diff)}</strong>
          </div>
          <div>
            <span>latest</span>
            <strong>{latest}</strong>
          </div>
        </div>

        <div className="binding-control">
          <select
            aria-label="Manual binding pane"
            value={paneID}
            onChange={(event) => setPaneID(event.target.value)}
            disabled={readonly || panes.length === 0}
          >
            {panes.length === 0 ? (
              <option value="">No panes</option>
            ) : (
              panes.map((pane) => (
                <option value={pane.pane_id} key={pane.pane_id}>
                  pane:{pane.pane_id} {pane.tab_title || pane.title || repoNameFromPath(pane.cwd) || pane.tty_name || "wezterm"}
                </option>
              ))
            )}
          </select>
          <button
            className="action-button"
            type="button"
            title={readonly ? "Readonly LAN view" : "Manually bind selected task to pane"}
            disabled={readonly || paneID === ""}
            onClick={() => onAttach(Number(paneID))}
          >
            <PanelRightOpen size={15} />
            <span>Bind</span>
          </button>
          <button
            className="icon-button"
            type="button"
            title={readonly ? "Readonly LAN view" : "Clear manual binding"}
            disabled={readonly || !manualBinding(task)}
            onClick={onDetach}
          >
            <RotateCcw size={15} />
          </button>
        </div>
      </div>

      <section className="inspector-dock">
        <nav className="inspector-tabs" aria-label="Inspector tabs">
          {inspectorTabs.map((tab) => (
            <button
              className={activeInspector === tab.id ? "active" : ""}
              type="button"
              key={tab.id}
              onClick={() => {
                onClosePanel();
                setActiveInspector(tab.id);
              }}
            >
              {tab.label}
            </button>
          ))}
        </nav>
        <div className="inspector-stack">
          {activeInspector === "overview" ? <OverviewPanel task={task} /> : null}
          {activeInspector === "diff" ? (
            <DiffRadarPanel
              diff={task.diff}
              expanded={expandedPanel === "diff"}
              closing={closingPanel === "diff"}
              onToggle={() => (expandedPanel === "diff" ? onClosePanel() : onExpandPanel("diff"))}
            />
          ) : null}
          {activeInspector === "trace" ? (
            <TracePanel
              spans={task.session.trace ?? []}
              latestAssistantAt={latestAssistantTime(task)}
              expanded={expandedPanel === "trace"}
              closing={closingPanel === "trace"}
              onToggle={() => (expandedPanel === "trace" ? onClosePanel() : onExpandPanel("trace"))}
            />
          ) : null}
          {activeInspector === "status" ? (
            <StatusExplanationPanel
              task={task}
              expanded={expandedPanel === "status"}
              closing={closingPanel === "status"}
              onToggle={() => (expandedPanel === "status" ? onClosePanel() : onExpandPanel("status"))}
            />
          ) : null}
          {activeInspector === "flight" ? (
            <FlightRecorder
              events={task.flight ?? eventsToFlight(events)}
              expanded={expandedPanel === "flight"}
              closing={closingPanel === "flight"}
              onToggle={() => (expandedPanel === "flight" ? onClosePanel() : onExpandPanel("flight"))}
            />
          ) : null}
          {activeInspector === "debrief" ? (
            <DebriefPanel
              task={task}
              expanded={expandedPanel === "debrief"}
              closing={closingPanel === "debrief"}
              onToggle={() => (expandedPanel === "debrief" ? onClosePanel() : onExpandPanel("debrief"))}
            />
          ) : null}
        </div>
      </section>
    </>
  );
}

function OverviewPanel({ task }: { task: Task }) {
  return (
    <div className="overview-panel instrument-panel">
      <div className="panel-head overview-head">
        <div className="panel-title">
          <strong>overview</strong>
          <span>selected task compact state</span>
        </div>
      </div>
      <div className="panel-body overview-body">
        <div className="kv overview-kv">
          <span className="k">agent</span><span className="v">{task.session.agent}</span>
          <span className="k">session</span><span className="v">{task.session.id}</span>
          <span className="k">binding</span><span className="v">{(task.binding.reasons ?? []).join(", ") || bindingLabel(task)}</span>
          <span className="k">confidence</span><span className="v">{confidenceLabel(task)}</span>
          <span className="k">cwd</span><span className="v">{task.session.cwd || "unknown"}</span>
          <span className="k">visibility</span><span className="v">{visibilityLabel(task)}</span>
          <span className="k">summary</span><span className="v">{task.summary || "no summary yet"}</span>
        </div>
      </div>
    </div>
  );
}

function SettingsPanel({
  settings,
  saving,
  onClose,
  onSave,
}: {
  settings: CockpitSettings;
  saving: boolean;
  onClose: () => void;
  onSave: (settings: CockpitSettings) => void;
}) {
  const [draft, setDraft] = useState<CockpitSettings>(settings);

  useEffect(() => {
    setDraft(settings);
  }, [settings]);

  const update = <K extends keyof CockpitSettings>(key: K, value: CockpitSettings[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const updateNumber = (key: NumericSettingKey, value: string) => {
    setDraft((current) => ({ ...current, [key]: Math.max(1, Number(value) || 1) }));
  };
  const updateRule = (ruleID: string, patch: Partial<AttentionRule>) => {
    setDraft((current) => ({
      ...current,
      attention_rules: (current.attention_rules ?? []).map((rule) => (rule.id === ruleID ? { ...rule, ...patch } : rule)),
    }));
  };

  return (
    <div className="settings-overlay" role="dialog" aria-modal="true" aria-label="Cockpit settings">
      <form
        className="settings-dialog"
        onSubmit={(event) => {
          event.preventDefault();
          onSave(draft);
        }}
      >
        <div className="settings-head">
          <div>
            <strong>settings</strong>
            <span>cockpit runtime controls</span>
          </div>
          <button className="icon-button" type="button" title="Close settings" onClick={onClose}>
            <X size={15} />
          </button>
        </div>

        <div className="settings-body">
          <section className="settings-section">
            <h3>identity</h3>
            <div className="settings-grid identity">
              <label>
                <span>cockpit owner</span>
                <input value={draft.cockpit_owner ?? ""} placeholder="owner name" onChange={(event) => update("cockpit_owner", event.target.value)} />
              </label>
            </div>
          </section>

          <section className="settings-section">
            <h3>status</h3>
            <div className="settings-grid">
              <label>
                <span>idle after</span>
                <input type="number" min={5} step={5} value={draft.idle_after_seconds} onChange={(event) => updateNumber("idle_after_seconds", event.target.value)} />
              </label>
              <label>
                <span>stuck after</span>
                <input type="number" min={30} step={30} value={draft.stuck_after_seconds} onChange={(event) => updateNumber("stuck_after_seconds", event.target.value)} />
              </label>
              <label>
                <span>recent hours</span>
                <input type="number" min={1} step={1} value={draft.recent_window_hours} onChange={(event) => updateNumber("recent_window_hours", event.target.value)} />
              </label>
              <label>
                <span>max sessions</span>
                <input type="number" min={1} step={1} value={draft.max_sessions} onChange={(event) => updateNumber("max_sessions", event.target.value)} />
              </label>
            </div>
          </section>

          <section className="settings-section">
            <h3>behavior</h3>
            <div className="settings-grid two">
              <label>
                <span>binding</span>
                <select value={draft.binding_mode} onChange={(event) => update("binding_mode", event.target.value === "manual" ? "manual" : "auto")}>
                  <option value="auto">auto</option>
                  <option value="manual">manual only</option>
                </select>
              </label>
              <label>
                <span>summary interval</span>
                <input type="number" min={10} step={10} value={draft.summary_interval_seconds} onChange={(event) => updateNumber("summary_interval_seconds", event.target.value)} />
              </label>
            </div>
            <div className="toggle-grid">
              <label className="toggle-row">
                <input type="checkbox" checked={draft.show_unbound} onChange={(event) => update("show_unbound", event.target.checked)} />
                <span>show unbound sessions</span>
              </label>
              <label className="toggle-row">
                <input type="checkbox" checked={draft.enable_llm_summary} onChange={(event) => update("enable_llm_summary", event.target.checked)} />
                <span>enable LLM summary</span>
              </label>
            </div>
          </section>

          <section className="settings-section">
            <h3>notifications</h3>
            <div className="settings-grid two">
              <label>
                <span>mode</span>
                <select value={draft.notification_mode} onChange={(event) => update("notification_mode", event.target.value === "focus" || event.target.value === "silent" ? event.target.value : "normal")}>
                  <option value="normal">normal</option>
                  <option value="focus">focus</option>
                  <option value="silent">silent</option>
                </select>
              </label>
              <label>
                <span>quiet hours</span>
                <div className="quiet-hours">
                  <input type="checkbox" checked={draft.quiet_hours_enabled} onChange={(event) => update("quiet_hours_enabled", event.target.checked)} />
                  <input value={draft.quiet_hours_start} onChange={(event) => update("quiet_hours_start", event.target.value)} />
                  <input value={draft.quiet_hours_end} onChange={(event) => update("quiet_hours_end", event.target.value)} />
                </div>
              </label>
            </div>
            <div className="toggle-grid">
              <label className="toggle-row">
                <input type="checkbox" checked={draft.notify_attention} onChange={(event) => update("notify_attention", event.target.checked)} />
                <span>needs decision</span>
              </label>
              <label className="toggle-row">
                <input type="checkbox" checked={draft.notify_input_sound} onChange={(event) => update("notify_input_sound", event.target.checked)} />
                <span>input sound</span>
              </label>
              <label className="toggle-row">
                <input type="checkbox" checked={draft.notify_completed} onChange={(event) => update("notify_completed", event.target.checked)} />
                <span>completed</span>
              </label>
              <label className="toggle-row">
                <input type="checkbox" checked={draft.notify_stuck} onChange={(event) => update("notify_stuck", event.target.checked)} />
                <span>stuck</span>
              </label>
            </div>
          </section>

          <section className="settings-section">
            <h3>attention rules</h3>
            <div className="rule-settings">
              {(draft.attention_rules ?? []).map((rule) => (
                <div className="rule-setting" key={rule.id}>
                  <label className="toggle-row">
                    <input type="checkbox" checked={rule.enabled} onChange={(event) => updateRule(rule.id, { enabled: event.target.checked })} />
                    <span>{rule.name}</span>
                  </label>
                  <select value={rule.severity} onChange={(event) => updateRule(rule.id, { severity: event.target.value === "blocked" || event.target.value === "drift" ? event.target.value : "attention" })}>
                    <option value="attention">attention</option>
                    <option value="blocked">blocked</option>
                    <option value="drift">drift</option>
                  </select>
                  <input value={rule.pattern} onChange={(event) => updateRule(rule.id, { pattern: event.target.value })} />
                </div>
              ))}
            </div>
          </section>

          <section className="settings-section">
            <h3>paths</h3>
            <div className="settings-grid paths">
              <label>
                <span>codex home</span>
                <input value={draft.codex_home} onChange={(event) => update("codex_home", event.target.value)} />
              </label>
              <label>
                <span>claude home</span>
                <input value={draft.claude_home} onChange={(event) => update("claude_home", event.target.value)} />
              </label>
              <label>
                <span>codex bin</span>
                <input value={draft.codex_bin} onChange={(event) => update("codex_bin", event.target.value)} />
              </label>
              <label>
                <span>wezterm bin</span>
                <input value={draft.wezterm_bin} onChange={(event) => update("wezterm_bin", event.target.value)} />
              </label>
            </div>
          </section>
        </div>

        <div className="settings-foot">
          <button className="action-button" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="action-button primary" type="submit" disabled={saving}>
            {saving ? "Saving" : "Save"}
          </button>
        </div>
      </form>
    </div>
  );
}

function StatusExplanationPanel({
  task,
  expanded,
  closing,
  onToggle,
}: {
  task: Task;
  expanded: boolean;
  closing: boolean;
  onToggle: () => void;
}) {
  const explanation = task.status_explanation ?? fallbackStatusExplanation(task);
  const background = explanation.background_run_traces ?? [];
  const evidence = explanation.evidence ?? [];
  return (
    <div className={`status-explain instrument-panel status-panel ${expanded ? "is-expanded" : ""} ${closing ? "is-closing" : ""}`}>
      <div className="explain-head panel-head">
        <div className="panel-title">
          <strong>status logic</strong>
          <span>{explanation.rule || "snapshot"}</span>
        </div>
        <button
          className="icon-button panel-toggle"
          type="button"
          title={expanded ? "Restore status logic" : "Expand status logic"}
          aria-label={expanded ? "Restore status logic" : "Expand status logic"}
          aria-expanded={expanded}
          onClick={onToggle}
        >
          {expanded ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
        </button>
      </div>
      <div className="panel-body status-panel-body">
        <div className="status-body">
          <div className="explain-primary">
            <span className={`status-pill ${statusClass(explanation.status || task.status)}`}>
              {statusLabel[explanation.status || task.status]}
            </span>
            <span>{explanation.reason || task.attention_reason || task.summary || "no explicit reason"}{explanation.rule_severity ? ` / ${explanation.rule_severity}` : ""}</span>
          </div>
          <div className="explain-grid">
            <div>
              <span className="explain-key">assistant</span>
              <strong>{timeOnly(explanation.latest_assistant_at)}</strong>
            </div>
            <div>
              <span className="explain-key">active</span>
              <strong>{traceRefLabel(explanation.active_trace)}</strong>
            </div>
            <div>
              <span className="explain-key">background</span>
              <strong>{background.length > 0 ? background.map(traceRefLabel).join(", ") : "none"}</strong>
            </div>
          </div>
          {explanation.why_not_working && (explanation.status || task.status) !== "working" ? (
            <div className="logic-strip">
              <span>not working</span>
              <strong>{explanation.why_not_working}</strong>
            </div>
          ) : null}
          {explanation.why_not_idle && (explanation.status || task.status) !== "idle" ? (
            <div className="logic-strip">
              <span>not idle</span>
              <strong>{explanation.why_not_idle}</strong>
            </div>
          ) : null}
          {background.length > 0 ? (
            <div className="background-traces">
              {background.slice(0, expanded ? 8 : 3).map((trace) => (
                <span key={trace.id}>{traceRefLabel(trace)}</span>
              ))}
            </div>
          ) : null}
          {explanation.attention_event ? (
            <div className="attention-source">
              <span>{timeOnly(explanation.attention_event.at)} {explanation.attention_event.type}</span>
              <strong>{eventText(explanation.attention_event)}</strong>
            </div>
          ) : null}
          {evidence.length > 0 ? (
            <div className="evidence-list">
              {evidence.slice(0, expanded ? 12 : 4).map((item, index) => (
                <span key={`${item}-${index}`}>{item}</span>
              ))}
            </div>
          ) : null}
        </div>
        {expanded ? <StatusLogicInspector task={task} explanation={explanation} /> : null}
      </div>
    </div>
  );
}

function TracePanel({
  spans,
  latestAssistantAt,
  expanded,
  closing,
  onToggle,
}: {
  spans: TraceSpan[];
  latestAssistantAt: number;
  expanded: boolean;
  closing: boolean;
  onToggle: () => void;
}) {
  const [filter, setFilter] = useState<TraceFilter>("active");
  const total = countTraceSpans(spans);
  const running = runningTraceBranches(spans, latestAssistantAt);
  const visible = expanded ? filteredTraceBranches(spans, filter, latestAssistantAt) : running.length > 0 ? running : recentTraceBranches(spans, 4, latestAssistantAt);
  const mode = expanded ? filter : running.length > 0 ? "active" : "recent";
  return (
    <div className={`trace-panel instrument-panel ${expanded ? "is-expanded" : ""} ${closing ? "is-closing" : ""}`}>
      <div className="trace-head panel-head">
        <div className="panel-title">
          <strong>trace</strong>
          <span>{mode} / {total} spans</span>
        </div>
        <div className="panel-actions">
          {expanded ? (
            <div className="trace-filter" aria-label="Trace filter">
              {traceFilterOptions.map((option) => (
                <button className={filter === option.id ? "active" : ""} type="button" key={option.id} onClick={() => setFilter(option.id)}>
                  {option.label}
                </button>
              ))}
            </div>
          ) : null}
          <button
            className="icon-button panel-toggle"
            type="button"
            title={expanded ? "Restore trace" : "Expand trace"}
            aria-label={expanded ? "Restore trace" : "Expand trace"}
            aria-expanded={expanded}
            onClick={onToggle}
          >
            {expanded ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
          </button>
        </div>
      </div>
      <div className="panel-body trace-body">
        {expanded ? <TraceMinimap spans={spans} latestAssistantAt={latestAssistantAt} /> : null}
        {visible.length === 0 ? (
          <div className="trace-empty">
            <GitBranch size={16} />
            <span>No structured trace parsed yet.</span>
          </div>
        ) : (
          <div className="trace-tree">
            {visible.map((span) => (
              <TraceNode span={span} depth={0} expanded={expanded} key={span.id} />
            ))}
          </div>
        )}
        {expanded ? <TraceInspector spans={spans} latestAssistantAt={latestAssistantAt} /> : null}
      </div>
    </div>
  );
}

function TraceMinimap({ spans, latestAssistantAt }: { spans: TraceSpan[]; latestAssistantAt: number }) {
  const flat = flattenTrace(spans).filter((span) => span.kind !== "turn");
  if (flat.length === 0) {
    return null;
  }
  return (
    <div className="trace-minimap" aria-hidden="true">
      {flat.slice(-80).map((span, index) => (
        <span
          className={`${traceKindClass(span.kind)} ${traceStatusClass(span.status)} ${isActiveRunningWorkSpan(span, latestAssistantAt) ? "active" : ""}`}
          title={`${span.kind} ${span.name}`}
          key={`${span.id}-${index}`}
        />
      ))}
    </div>
  );
}

function TraceNode({ span, depth, expanded }: { span: TraceSpan; depth: number; expanded: boolean }) {
  const children = span.children ?? [];
  return (
    <div className="trace-node">
      <div className={`trace-row ${traceKindClass(span.kind)}`} style={{ paddingLeft: 10 + depth * 18 }}>
        <span className={`trace-status ${traceStatusClass(span.status)}`}>{traceStatusIcon(span.status)}</span>
        {expanded ? <span className="trace-time">{traceTimestamp(span)}</span> : null}
        <div className="trace-main">
          <div className="trace-title-line">
            <span className="trace-kind">{traceKindLabel(span)}</span>
            <span className="trace-name">{traceTitle(span)}</span>
          </div>
          {traceDetail(span) ? <div className="trace-summary">{traceDetail(span)}</div> : null}
        </div>
        <span className="trace-duration">{traceDuration(span)}</span>
      </div>
      {children.map((child) => (
        <TraceNode span={child} depth={depth + 1} expanded={expanded} key={child.id} />
      ))}
    </div>
  );
}

function TraceInspector({ spans, latestAssistantAt }: { spans: TraceSpan[]; latestAssistantAt: number }) {
  const flat = flattenTrace(spans);
  const active = flat.find((span) => isActiveRunningWorkSpan(span, latestAssistantAt));
  const failed = [...flat].filter((span) => span.status === "failed").sort((a, b) => traceTime(b) - traceTime(a))[0];
  const latest = [...flat].sort((a, b) => traceTime(b) - traceTime(a))[0];
  const selected = active ?? failed ?? latest;
  const runningCount = flat.filter((span) => span.status === "running").length;
  const failedCount = flat.filter((span) => span.status === "failed").length;
  return (
    <aside className="trace-inspector">
      <div className="inspector-section">
        <h3>selected span</h3>
        {selected ? (
          <>
            <div className="inspector-kv">
              <span>kind</span><strong>{traceKindLabel(selected)}</strong>
              <span>name</span><strong>{traceTitle(selected)}</strong>
              <span>status</span><strong>{selected.status}</strong>
              <span>time</span><strong>{traceTimestamp(selected)}</strong>
              <span>duration</span><strong>{traceDuration(selected) || "n/a"}</strong>
            </div>
            {traceDetail(selected) ? <p>{traceDetail(selected)}</p> : null}
          </>
        ) : (
          <p>No structured trace has been parsed for this session yet.</p>
        )}
      </div>
      <div className="inspector-section">
        <h3>counts</h3>
        <div className="inspector-kv">
          <span>total</span><strong>{flat.length}</strong>
          <span>running</span><strong>{runningCount}</strong>
          <span>failed</span><strong>{failedCount}</strong>
          <span>assistant</span><strong>{latestAssistantAt > 0 ? timeOnly(new Date(latestAssistantAt).toISOString()) : "--:--:--"}</strong>
        </div>
      </div>
      <div className="inspector-section">
        <h3>display policy</h3>
        <p>Collapsed mode shows active branches, or the latest recent spans when nothing is active. Expanded mode can narrow the tree by activity, failures, tools, assistant replies, or all spans.</p>
      </div>
      {selected?.detail ? (
        <div className="inspector-section">
          <h3>detail</h3>
          <pre className="output-block">{selected.detail}</pre>
        </div>
      ) : null}
    </aside>
  );
}

function StatusLogicInspector({ task, explanation }: { task: Task; explanation: StatusExplanation }) {
  const events = [...(task.session.events ?? [])].slice(-8).reverse();
  const background = explanation.background_run_traces ?? [];
  return (
    <aside className="logic-inspector">
      <div className="inspector-section">
        <h3>working decision</h3>
        <div className="inspector-kv">
          <span>active</span><strong>{traceRefLabel(explanation.active_trace)}</strong>
          <span>not working</span><strong>{explanation.why_not_working || ((explanation.status || task.status) === "working" ? "active trace is blocking" : "not recorded")}</strong>
          <span>background</span><strong>{background.length > 0 ? background.map(traceRefLabel).join("\n") : "none"}</strong>
        </div>
      </div>
      {explanation.attention_event ? (
        <div className="inspector-section">
          <h3>attention source</h3>
          <pre className="output-block">{eventLine(explanation.attention_event)}</pre>
        </div>
      ) : null}
      <div className="inspector-section">
        <h3>rule ladder</h3>
        <div className="rule-stack">
          {statusRules(task, explanation).map((rule, index) => (
            <div className={`rule ${rule.active ? "active" : ""}`} key={rule.title}>
              <div className="rule-step">{String(index + 1).padStart(2, "0")}</div>
              <div>
                <strong>{rule.title}</strong>
                <span>{rule.body}</span>
              </div>
            </div>
          ))}
        </div>
      </div>
      <div className="inspector-section">
        <h3>evidence</h3>
        <pre className="output-block">{(explanation.evidence ?? []).length > 0 ? (explanation.evidence ?? []).join("\n") : "no explicit evidence"}</pre>
      </div>
      <div className="inspector-section">
        <h3>recent events</h3>
        <pre className="output-block">{events.length > 0 ? events.map(eventLine).join("\n") : "no recent events parsed"}</pre>
      </div>
    </aside>
  );
}

function displayTasks(snapshot: Snapshot, showHidden: boolean, showUnbound = true) {
  return snapshot.tasks.filter((task) => {
    if (task.session.internal) {
      return false;
    }
    if (!showUnbound && task.status === "unbound") {
      return false;
    }
    return showHidden || !isHidden(task);
  });
}

function isHidden(task: Task) {
  return Boolean(task.archived || task.ignored);
}

function hasInputAlert(task: Task) {
  return Boolean(validRecentDate(task.input_alert_at, 15 * 60_000));
}

function hiddenCount(snapshot: Snapshot) {
  return snapshot.tasks.filter((task) => !task.session.internal && isHidden(task)).length;
}

function collectStats(snapshot: Snapshot | null, showUnbound: boolean) {
  const tasks = snapshot ? displayTasks(snapshot, false, showUnbound) : [];
  return {
    attention: tasks.filter((task) => task.status === "needs_attention").length,
    waiting: tasks.filter((task) => task.status === "waiting").length,
    blocked: tasks.filter((task) => task.status === "blocked").length,
    drift: tasks.filter((task) => task.status === "drift").length,
    working: tasks.filter((task) => task.status === "working").length,
    idle: tasks.filter((task) => task.status === "idle").length,
    bound: tasks.filter((task) => Boolean(task.binding.pane)).length,
    hidden: snapshot ? hiddenCount(snapshot) : 0,
  };
}

function statusSeverity(status: Status) {
  switch (status) {
    case "blocked":
      return 90;
    case "needs_attention":
      return 80;
    case "waiting":
      return 72;
    case "drift":
      return 65;
    case "working":
      return 40;
    default:
      return 0;
  }
}

function radarPoint(index: number, total: number) {
  const angle = (index * 137.5 + 18) * (Math.PI / 180);
  const radius = 18 + ((index % Math.max(1, total)) / Math.max(1, total)) * 28;
  return {
    x: 50 + Math.cos(angle) * radius,
    y: 50 + Math.sin(angle) * radius,
  };
}

function missionCallsign(mission: Mission) {
  const clean = mission.name.replace(/[^a-z0-9]/gi, "").toUpperCase();
  return clean.slice(0, 11) || "MISSION";
}

function callsign(task: Task) {
  const repo = repoName(task).replace(/[^a-z0-9]/gi, "").toUpperCase();
  const prefix = task.session.agent === "claude" ? "CLD" : "CDX";
  return `${prefix}-${repo.slice(0, 8) || "TASK"}`;
}

function diffSummaryLabel(diff?: DiffRadar) {
  if (!diff) {
    return "unknown";
  }
  if (diff.last_error) {
    return "error";
  }
  if (!diff.dirty) {
    return "clean";
  }
  const changed = (diff.files ?? []).length;
  return `${changed} files, ${diff.tests} tests`;
}

function diffHeatClass(path: string, status: string) {
  const lower = path.toLowerCase();
  if (lower.includes("lock") || lower.endsWith("go.sum") || lower.includes("package-lock")) {
    return "lock";
  }
  if (lower.includes("test") || lower.includes("spec") || lower.endsWith("_test.go")) {
    return "test";
  }
  if (status === "added" || status === "untracked") {
    return "add";
  }
  if (status === "deleted") {
    return "del";
  }
  return "mod";
}

function visibilityLabel(task: Task) {
  if (task.archived) {
    return "archived";
  }
  if (task.ignored) {
    return "ignored";
  }
  return "visible";
}

function repoName(task: Task) {
  return repoNameFromPath(task.session.cwd);
}

function repoNameFromPath(path?: string) {
  const cwd = path?.replace(/\/+$/, "");
  if (!cwd) {
    return "unknown";
  }
  return cwd.split("/").pop() || cwd;
}

async function copyText(text: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "true");
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.select();
  document.execCommand("copy");
  document.body.removeChild(textarea);
}

function shortID(task: Task) {
  return task.session.id.length > 14 ? task.session.id.slice(0, 14) : task.session.id;
}

function manualBinding(task: Task) {
  return Boolean(task.binding.reasons?.includes("manual attach"));
}

function bindingLabel(task: Task) {
  const pane = task.binding.pane;
  if (!pane) {
    return "unbound";
  }
  return `${manualBinding(task) ? "manual" : "pane"}:${pane.pane_id}`;
}

function statusRules(task: Task, explanation: StatusExplanation) {
  return [
    {
      title: "binding",
      active: explanation.rule === "binding",
      body: task.binding.pane ? `bound to ${bindingLabel(task)} with ${confidenceLabel(task)} confidence` : "no reliable WezTerm pane binding",
    },
    {
      title: "active trace",
      active: explanation.rule === "trace_active",
      body: explanation.active_trace ? `blocking trace: ${traceRefLabel(explanation.active_trace)}` : explanation.why_not_working || "no blocking tool or subagent trace is active",
    },
    {
      title: "attention pattern",
      active: explanation.rule === "attention_pattern",
      body: explanation.attention_event ? `triggered by ${eventLine(explanation.attention_event)}` : task.attention_reason || (explanation.evidence ?? [])[0] || "no failure or decision pattern won",
    },
    {
      title: "assistant question",
      active: explanation.rule === "assistant_question",
      body: "latest assistant message is checked for a direct user decision prompt",
    },
    {
      title: "completion phrase",
      active: explanation.rule === "completion_phrase",
      body: "latest assistant message is checked for done, completed, finished, or Chinese completion wording",
    },
    {
      title: "idle timeout",
      active: explanation.rule === "idle_timeout",
      body: explanation.rule === "idle_timeout" ? explanation.reason || "settle window elapsed" : "assistant responses settle after 30s; other events use the global idle threshold",
    },
    {
      title: "recent activity",
      active: explanation.rule === "recent_activity",
      body: "fresh significant activity keeps the task in working state until it settles",
    },
  ];
}

function eventLine(event: SessionEvent) {
  const detail = event.detail ? ` ${event.detail}` : "";
  return `${timeOnly(event.at)} ${event.type}: ${event.text}${detail}`;
}

function eventText(event: SessionEvent) {
  return event.detail ? `${event.text} ${event.detail}` : event.text;
}

function fallbackStatusExplanation(task: Task): StatusExplanation {
  const latestAssistantAt = latestAssistantTime(task);
  const latestAssistant = latestAssistantAt > 0 ? new Date(latestAssistantAt).toISOString() : undefined;
  const background = backgroundTraceRefs(task.session.trace ?? [], latestAssistantAt);
  return {
    status: task.status,
    rule: "snapshot",
    reason: task.attention_reason || task.summary,
    why_not_working: task.status === "working" ? undefined : background.length > 0 ? `${background.length} running background trace(s) after assistant response` : "no active blocking trace in fallback snapshot",
    latest_assistant_at: latestAssistant,
    last_event_at: task.session.last_event_at,
    active_trace: runningTraceRefs(task.session.trace ?? [], latestAssistantAt)[0],
    background_run_traces: background,
    attention_event: latestAssistantEvent(task),
    evidence: task.attention_reason ? [task.attention_reason] : [],
  };
}

function runningTraceRefs(spans: TraceSpan[], latestAssistantAt: number): TraceRef[] {
  return flattenTrace(spans)
    .filter((span) => isActiveRunningWorkSpan(span, latestAssistantAt))
    .map(traceRefFromSpan);
}

function backgroundTraceRefs(spans: TraceSpan[], latestAssistantAt: number): TraceRef[] {
  if (latestAssistantAt === 0) {
    return [];
  }
  return flattenTrace(spans)
    .filter((span) => isBackgroundRunningTool(span, latestAssistantAt))
    .map(traceRefFromSpan);
}

function traceRefFromSpan(span: TraceSpan): TraceRef {
  return {
    id: span.id,
    kind: span.kind,
    name: span.name,
    started_at: span.started_at,
    summary: span.summary,
    detail: span.detail,
  };
}

function traceRefLabel(trace?: TraceRef) {
  if (!trace) {
    return "none";
  }
  const detail = trace.detail || trace.summary;
  return detail ? `${trace.kind}:${trace.name} ${detail}` : `${trace.kind}:${trace.name}`;
}

function confidenceLabel(task: Task) {
  if (!task.binding.pane) {
    return "none";
  }
  if (task.binding.confidence >= 999) {
    return "manual";
  }
  return `${task.binding.confidence}%`;
}

function statusClass(status: Status) {
  switch (status) {
    case "needs_attention":
      return "attn";
    case "waiting":
      return "wait";
    case "blocked":
      return "blocked";
    case "drift":
      return "drift";
    case "working":
      return "work";
    case "idle":
      return "idle";
    case "completed":
      return "done";
    case "unbound":
      return "link";
    default:
      return "unknown";
  }
}

function relativeAge(raw?: string) {
  const date = validDate(raw);
  if (!date) {
    return "unknown";
  }
  const ms = Date.now() - date.getTime();
  if (ms < 60_000) {
    return `${Math.max(0, Math.round(ms / 1000))}s ago`;
  }
  if (ms < 60 * 60_000) {
    return `${Math.round(ms / 60_000)}m ago`;
  }
  return `${Math.round(ms / (60 * 60_000))}h ago`;
}

function timeOnly(raw?: string) {
  const date = validDate(raw);
  if (!date) {
    return "--:--:--";
  }
  return date.toLocaleTimeString([], { hour12: false });
}

function validRecentDate(raw: string | undefined, maxAgeMs: number) {
  const date = validDate(raw);
  if (!date) {
    return null;
  }
  const age = Date.now() - date.getTime();
  if (age < -30_000 || age > maxAgeMs) {
    return null;
  }
  return date;
}

function validDate(raw?: string) {
  if (!raw) {
    return null;
  }
  const date = new Date(raw);
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 2020) {
    return null;
  }
  return date;
}

function traceStatusClass(status: TraceStatus) {
  switch (status) {
    case "running":
      return "running";
    case "succeeded":
      return "ok";
    case "failed":
      return "fail";
    default:
      return "unknown";
  }
}

function traceStatusIcon(status: TraceStatus) {
  switch (status) {
    case "running":
      return <LoaderCircle size={13} className="spin" />;
    case "succeeded":
      return <CheckCircle2 size={13} />;
    case "failed":
      return <XCircle size={13} />;
    default:
      return <CircleDot size={13} />;
  }
}

function traceKindClass(kind: TraceSpan["kind"]) {
  return `trace-${kind}`;
}

function traceKindLabel(span: TraceSpan) {
  switch (span.kind) {
    case "assistant":
      return "REPLY";
    case "subagent":
      return "WORKER";
    case "tool":
      return "TOOL";
    case "turn":
      return "TURN";
    default:
      return String(span.kind).toUpperCase();
  }
}

function traceTitle(span: TraceSpan) {
  switch (span.kind) {
    case "assistant":
      return "assistant response";
    case "subagent":
      return span.name === "spawn_agent" ? "subagent" : span.name;
    case "turn":
      return span.summary || span.name || "user turn";
    default:
      return span.name || span.kind;
  }
}

function traceDetail(span: TraceSpan) {
  if (span.kind === "turn") {
    return span.name === span.summary ? "" : span.summary || "";
  }
  return span.detail || span.summary || "";
}

function traceDuration(span: TraceSpan) {
  if (span.status === "running" && span.started_at) {
    return relativeAge(span.started_at).replace(" ago", "");
  }
  if (span.duration_millis !== undefined && span.duration_millis > 0) {
    if (span.duration_millis < 1000) {
      return `${span.duration_millis}ms`;
    }
    return `${(span.duration_millis / 1000).toFixed(span.duration_millis < 10_000 ? 1 : 0)}s`;
  }
  if (span.ended_at && span.started_at) {
    const ms = new Date(span.ended_at).getTime() - new Date(span.started_at).getTime();
    if (Number.isFinite(ms) && ms > 0) {
      return `${Math.round(ms / 1000)}s`;
    }
  }
  return "";
}

function traceTimestamp(span: TraceSpan) {
  const start = timeOnly(span.started_at);
  const end = span.ended_at ? timeOnly(span.ended_at) : span.status === "running" ? "running" : "";
  if (end && end !== start) {
    return `${start} -> ${end}`;
  }
  return start;
}

function countTraceSpans(spans: TraceSpan[]): number {
  return spans.reduce((total, span) => total + 1 + countTraceSpans(span.children ?? []), 0);
}

function runningTraceBranches(spans: TraceSpan[], latestAssistantAt: number): TraceSpan[] {
  return spans.flatMap((span) => {
    const children = runningTraceBranches(span.children ?? [], latestAssistantAt);
    if (isActiveRunningWorkSpan(span, latestAssistantAt) || children.length > 0) {
      return [{ ...span, children }];
    }
    return [];
  });
}

function filteredTraceBranches(spans: TraceSpan[], filter: TraceFilter, latestAssistantAt: number): TraceSpan[] {
  switch (filter) {
    case "active": {
      const active = runningTraceBranches(spans, latestAssistantAt);
      return active.length > 0 ? active : recentTraceBranches(spans, 8, latestAssistantAt);
    }
    case "failed":
      return matchingTraceBranches(spans, (span) => span.status === "failed");
    case "tools":
      return matchingTraceBranches(spans, (span) => span.kind === "tool" || span.kind === "subagent");
    case "assistant":
      return matchingTraceBranches(spans, (span) => span.kind === "assistant");
    case "all":
    default:
      return spans;
  }
}

function matchingTraceBranches(spans: TraceSpan[], predicate: (span: TraceSpan) => boolean): TraceSpan[] {
  return spans.flatMap((span) => {
    const ownMatch = predicate(span);
    const children = matchingTraceBranches(span.children ?? [], predicate);
    if (!ownMatch && children.length === 0) {
      return [];
    }
    return [{ ...span, children: ownMatch ? span.children ?? [] : children }];
  });
}

function recentTraceBranches(spans: TraceSpan[], limit: number, latestAssistantAt: number): TraceSpan[] {
  const leaves = flattenTrace(spans)
    .filter((span) => span.kind !== "turn" && !isBackgroundRunningTool(span, latestAssistantAt))
    .sort((a, b) => traceTime(b) - traceTime(a))
    .slice(0, limit)
    .reverse();
  return leaves.map((span) => ({ ...span, children: [] }));
}

function flattenTrace(spans: TraceSpan[]): TraceSpan[] {
  return spans.flatMap((span) => [span, ...flattenTrace(span.children ?? [])]);
}

function isActiveRunningWorkSpan(span: TraceSpan, latestAssistantAt: number) {
  if (span.status !== "running") {
    return false;
  }
  if (span.kind === "subagent") {
    return true;
  }
  if (span.kind !== "tool") {
    return false;
  }
  const startedAt = timeValue(span.started_at);
  return latestAssistantAt === 0 || startedAt === 0 || latestAssistantAt <= startedAt;
}

function isBackgroundRunningTool(span: TraceSpan, latestAssistantAt: number) {
  const startedAt = timeValue(span.started_at);
  return span.status === "running" && span.kind === "tool" && latestAssistantAt > 0 && startedAt > 0 && latestAssistantAt > startedAt;
}

function traceTime(span: TraceSpan) {
  const raw = span.ended_at || span.started_at;
  return timeValue(raw);
}

function latestAssistantTime(task: Task) {
  const eventTime = [...(task.session.events ?? [])].reverse().find((event) => event.type === "assistant")?.at;
  return Math.max(timeValue(eventTime), latestAssistantTraceTime(task.session.trace ?? []));
}

function latestAssistantEvent(task: Task) {
  return [...(task.session.events ?? [])].reverse().find((event) => event.type === "assistant");
}

function latestAssistantTraceTime(spans: TraceSpan[]): number {
  return spans.reduce((latest, span) => {
    const own = span.kind === "assistant" ? timeValue(span.ended_at || span.started_at) : 0;
    return Math.max(latest, own, latestAssistantTraceTime(span.children ?? []));
  }, 0);
}

function eventsToFlight(events: SessionEvent[]): FlightEvent[] {
  return events.map((event, index) => ({
    id: `fallback:${index}`,
    at: event.at,
    kind: event.type,
    title: event.type,
    detail: eventText(event),
    source: "log",
  }));
}

function commandMatches(tasks: Task[], missions: Mission[], query: string) {
  const needle = query.trim().toLowerCase();
  const taskMatches = tasks
    .filter((task) => {
      if (!needle) {
        return true;
      }
      return [task.id, task.session.id, task.session.title, task.session.cwd, task.summary, task.status, repoName(task)]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(needle));
    })
    .map((task) => ({ kind: "task" as const, id: task.id, task }));
  const missionMatches = missions
    .filter((mission) => {
      if (!needle) {
        return true;
      }
      return [mission.name, mission.cwd, mission.repo_root, mission.summary, mission.status]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(needle));
    })
    .map((mission) => ({
      kind: "mission" as const,
      id: mission.id,
      title: mission.name,
      detail: `${mission.task_ids.length} tasks / ${mission.changed_files} files / ${mission.status}`,
      taskID: mission.task_ids[0],
    }));
  return [...taskMatches, ...missionMatches];
}

function timeValue(raw?: string) {
  if (!raw) {
    return 0;
  }
  const value = new Date(raw).getTime();
  return Number.isFinite(value) ? value : 0;
}

export default App;
