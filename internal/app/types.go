package app

import (
	"path/filepath"
	"strings"
	"time"
)

type Agent string

const (
	AgentCodex  Agent = "codex"
	AgentClaude Agent = "claude"
)

type EventType string

const (
	EventUser      EventType = "user"
	EventAssistant EventType = "assistant"
	EventTool      EventType = "tool"
	EventResult    EventType = "result"
	EventSystem    EventType = "system"
	EventError     EventType = "error"
)

type Status string

const (
	StatusNeedsAttention Status = "needs_attention"
	StatusWaiting        Status = "waiting"
	StatusWorking        Status = "working"
	StatusDrift          Status = "drift"
	StatusBlocked        Status = "blocked"
	StatusIdle           Status = "idle"
	StatusCompleted      Status = "completed"
	StatusUnbound        Status = "unbound"
	StatusUnknown        Status = "unknown"
)

type Event struct {
	At     time.Time `json:"at" ts_type:"string"`
	Type   EventType `json:"type"`
	Text   string    `json:"text"`
	Detail string    `json:"detail,omitempty"`
}

type TraceKind string

const (
	TraceTurn      TraceKind = "turn"
	TraceAssistant TraceKind = "assistant"
	TraceTool      TraceKind = "tool"
	TraceSubagent  TraceKind = "subagent"
)

type TraceStatus string

const (
	TraceRunning   TraceStatus = "running"
	TraceSucceeded TraceStatus = "succeeded"
	TraceFailed    TraceStatus = "failed"
	TraceUnknown   TraceStatus = "unknown"
)

type TraceSpan struct {
	ID             string      `json:"id"`
	ParentID       string      `json:"parent_id,omitempty"`
	Kind           TraceKind   `json:"kind"`
	Name           string      `json:"name"`
	Status         TraceStatus `json:"status"`
	StartedAt      time.Time   `json:"started_at,omitempty" ts_type:"string"`
	EndedAt        time.Time   `json:"ended_at,omitempty" ts_type:"string"`
	DurationMillis int64       `json:"duration_millis,omitempty"`
	Summary        string      `json:"summary,omitempty"`
	Detail         string      `json:"detail,omitempty"`
	ExitCode       *int        `json:"exit_code,omitempty"`
	Children       []TraceSpan `json:"children,omitempty"`
}

type TraceRef struct {
	ID        string    `json:"id"`
	Kind      TraceKind `json:"kind"`
	Name      string    `json:"name"`
	StartedAt time.Time `json:"started_at,omitempty" ts_type:"string"`
	Summary   string    `json:"summary,omitempty"`
	Detail    string    `json:"detail,omitempty"`
}

type StatusExplanation struct {
	Status              Status     `json:"status"`
	Rule                string     `json:"rule,omitempty"`
	RuleSeverity        string     `json:"rule_severity,omitempty"`
	Reason              string     `json:"reason,omitempty"`
	WhyNotWorking       string     `json:"why_not_working,omitempty"`
	WhyNotIdle          string     `json:"why_not_idle,omitempty"`
	LatestAssistantAt   time.Time  `json:"latest_assistant_at,omitempty" ts_type:"string"`
	LastEventAt         time.Time  `json:"last_event_at,omitempty" ts_type:"string"`
	ActiveTrace         *TraceRef  `json:"active_trace,omitempty"`
	BackgroundRunTraces []TraceRef `json:"background_run_traces,omitempty"`
	AttentionEvent      *Event     `json:"attention_event,omitempty"`
	Evidence            []string   `json:"evidence,omitempty"`
}

type DiffFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions,omitempty"`
	Deletions int    `json:"deletions,omitempty"`
}

type DiffRadar struct {
	RepoRoot    string     `json:"repo_root,omitempty"`
	Branch      string     `json:"branch,omitempty"`
	Dirty       bool       `json:"dirty"`
	Files       []DiffFile `json:"files,omitempty"`
	Added       int        `json:"added"`
	Modified    int        `json:"modified"`
	Deleted     int        `json:"deleted"`
	Renamed     int        `json:"renamed"`
	Untracked   int        `json:"untracked"`
	Tests       int        `json:"tests"`
	Lockfiles   int        `json:"lockfiles"`
	Summary     string     `json:"summary,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	GeneratedAt time.Time  `json:"generated_at,omitempty" ts_type:"string"`
}

type FlightEvent struct {
	ID             string    `json:"id"`
	ParentID       string    `json:"parent_id,omitempty"`
	At             time.Time `json:"at,omitempty" ts_type:"string"`
	Kind           string    `json:"kind"`
	Title          string    `json:"title"`
	Detail         string    `json:"detail,omitempty"`
	Status         string    `json:"status,omitempty"`
	DurationMillis int64     `json:"duration_millis,omitempty"`
	Source         string    `json:"source,omitempty"`
}

type Debrief struct {
	TaskID      string    `json:"task_id"`
	SessionID   string    `json:"session_id"`
	GeneratedAt time.Time `json:"generated_at" ts_type:"string"`
	Text        string    `json:"text"`
	InputHash   string    `json:"input_hash"`
	Raw         string    `json:"raw,omitempty"`
}

type Session struct {
	ID             string      `json:"id"`
	Agent          Agent       `json:"agent"`
	CWD            string      `json:"cwd,omitempty"`
	LogPath        string      `json:"log_path,omitempty"`
	Title          string      `json:"title,omitempty"`
	LastPrompt     string      `json:"last_prompt,omitempty"`
	CreatedAt      time.Time   `json:"created_at,omitempty" ts_type:"string"`
	LastEventAt    time.Time   `json:"last_event_at,omitempty" ts_type:"string"`
	PID            int         `json:"pid,omitempty"`
	ProcessUUID    string      `json:"process_uuid,omitempty"`
	Internal       bool        `json:"internal,omitempty"`
	InternalReason string      `json:"internal_reason,omitempty"`
	Events         []Event     `json:"events,omitempty"`
	Trace          []TraceSpan `json:"trace,omitempty"`
}

type Pane struct {
	WindowID    int    `json:"window_id"`
	TabID       int    `json:"tab_id"`
	PaneID      int    `json:"pane_id"`
	Workspace   string `json:"workspace,omitempty"`
	Title       string `json:"title,omitempty"`
	TabTitle    string `json:"tab_title,omitempty"`
	WindowTitle string `json:"window_title,omitempty"`
	CWD         string `json:"cwd,omitempty"`
	TTYName     string `json:"tty_name,omitempty"`
	IsActive    bool   `json:"is_active,omitempty"`
}

type Process struct {
	PID  int    `json:"pid"`
	PPID int    `json:"ppid"`
	TTY  string `json:"tty,omitempty"`
	Args string `json:"args,omitempty"`
}

type Binding struct {
	Pane       *Pane    `json:"pane,omitempty"`
	Process    *Process `json:"process,omitempty"`
	Confidence int      `json:"confidence"`
	Reasons    []string `json:"reasons,omitempty"`
}

type Task struct {
	ID              string            `json:"id"`
	Session         Session           `json:"session"`
	Binding         Binding           `json:"binding"`
	Status          Status            `json:"status"`
	StatusExplain   StatusExplanation `json:"status_explanation,omitempty"`
	AttentionReason string            `json:"attention_reason,omitempty"`
	Summary         string            `json:"summary,omitempty"`
	SummaryAt       time.Time         `json:"summary_at,omitempty" ts_type:"string"`
	Diff            DiffRadar         `json:"diff,omitempty"`
	Flight          []FlightEvent     `json:"flight,omitempty"`
	Debrief         *Debrief          `json:"debrief,omitempty"`
	Archived        bool              `json:"archived,omitempty"`
	Ignored         bool              `json:"ignored,omitempty"`
}

type DoneInboxItem struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	SessionID   string    `json:"session_id"`
	Agent       Agent     `json:"agent"`
	Project     string    `json:"project"`
	CWD         string    `json:"cwd,omitempty"`
	Title       string    `json:"title,omitempty"`
	Status      Status    `json:"status"`
	Summary     string    `json:"summary,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty" ts_type:"string"`
	Reviewed    bool      `json:"reviewed,omitempty"`
	Archived    bool      `json:"archived,omitempty"`
	PaneID      *int      `json:"pane_id,omitempty"`
	Diff        DiffRadar `json:"diff,omitempty"`
	Debrief     *Debrief  `json:"debrief,omitempty"`
}

type Mission struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	CWD          string    `json:"cwd,omitempty"`
	RepoRoot     string    `json:"repo_root,omitempty"`
	Status       Status    `json:"status"`
	TaskIDs      []string  `json:"task_ids"`
	Attention    int       `json:"attention"`
	Working      int       `json:"working"`
	Drift        int       `json:"drift"`
	Blocked      int       `json:"blocked"`
	Idle         int       `json:"idle"`
	Completed    int       `json:"completed"`
	ChangedFiles int       `json:"changed_files"`
	LastEventAt  time.Time `json:"last_event_at,omitempty" ts_type:"string"`
	Summary      string    `json:"summary,omitempty"`
}

type Snapshot struct {
	BuiltAt   time.Time       `json:"built_at" ts_type:"string"`
	Tasks     []Task          `json:"tasks"`
	Panes     []Pane          `json:"panes"`
	Processes []Process       `json:"processes"`
	Missions  []Mission       `json:"missions,omitempty"`
	DoneInbox []DoneInboxItem `json:"done_inbox,omitempty"`
	Errors    []string        `json:"errors,omitempty"`
}

func TaskID(session Session) string {
	return string(session.Agent) + ":" + session.ID
}

func (t Task) IDShort() string {
	id := strings.TrimPrefix(t.ID, string(t.Session.Agent)+":")
	if len(id) > 14 {
		return id[:14]
	}
	return id
}

func (t Task) RepoName() string {
	if t.Session.CWD == "" {
		return "unknown"
	}
	base := filepath.Base(t.Session.CWD)
	if base == "." || base == "/" || base == "" {
		return t.Session.CWD
	}
	return base
}

func StatusRank(status Status) int {
	switch status {
	case StatusNeedsAttention:
		return 0
	case StatusWaiting:
		return 1
	case StatusBlocked:
		return 2
	case StatusDrift:
		return 3
	case StatusWorking:
		return 4
	case StatusIdle:
		return 5
	case StatusUnbound:
		return 6
	case StatusCompleted:
		return 7
	default:
		return 9
	}
}

func HasRunningWorkTrace(spans []TraceSpan) bool {
	for _, span := range spans {
		if span.Status == TraceRunning && (span.Kind == TraceTool || span.Kind == TraceSubagent) {
			return true
		}
		if HasRunningWorkTrace(span.Children) {
			return true
		}
	}
	return false
}

func RunningTraceName(spans []TraceSpan) string {
	for _, span := range spans {
		if span.Status == TraceRunning && (span.Kind == TraceTool || span.Kind == TraceSubagent) {
			return span.Name
		}
		if name := RunningTraceName(span.Children); name != "" {
			return name
		}
	}
	return ""
}
