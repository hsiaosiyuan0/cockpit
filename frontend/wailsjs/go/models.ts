export namespace app {
	
	export class Process {
	    pid: number;
	    ppid: number;
	    tty?: string;
	    args?: string;
	
	    static createFrom(source: any = {}) {
	        return new Process(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pid = source["pid"];
	        this.ppid = source["ppid"];
	        this.tty = source["tty"];
	        this.args = source["args"];
	    }
	}
	export class Pane {
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
	
	    static createFrom(source: any = {}) {
	        return new Pane(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.window_id = source["window_id"];
	        this.tab_id = source["tab_id"];
	        this.pane_id = source["pane_id"];
	        this.workspace = source["workspace"];
	        this.title = source["title"];
	        this.tab_title = source["tab_title"];
	        this.window_title = source["window_title"];
	        this.cwd = source["cwd"];
	        this.tty_name = source["tty_name"];
	        this.is_active = source["is_active"];
	    }
	}
	export class Binding {
	    pane?: Pane;
	    process?: Process;
	    confidence: number;
	    reasons?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Binding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pane = this.convertValues(source["pane"], Pane);
	        this.process = this.convertValues(source["process"], Process);
	        this.confidence = source["confidence"];
	        this.reasons = source["reasons"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Debrief {
	    task_id: string;
	    session_id: string;
	    generated_at: string;
	    text: string;
	    input_hash: string;
	    raw?: string;
	
	    static createFrom(source: any = {}) {
	        return new Debrief(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.session_id = source["session_id"];
	        this.generated_at = source["generated_at"];
	        this.text = source["text"];
	        this.input_hash = source["input_hash"];
	        this.raw = source["raw"];
	    }
	}
	export class DiffFile {
	    path: string;
	    status: string;
	    additions?: number;
	    deletions?: number;
	
	    static createFrom(source: any = {}) {
	        return new DiffFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	        this.additions = source["additions"];
	        this.deletions = source["deletions"];
	    }
	}
	export class DiffRadar {
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
	
	    static createFrom(source: any = {}) {
	        return new DiffRadar(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo_root = source["repo_root"];
	        this.branch = source["branch"];
	        this.dirty = source["dirty"];
	        this.files = this.convertValues(source["files"], DiffFile);
	        this.added = source["added"];
	        this.modified = source["modified"];
	        this.deleted = source["deleted"];
	        this.renamed = source["renamed"];
	        this.untracked = source["untracked"];
	        this.tests = source["tests"];
	        this.lockfiles = source["lockfiles"];
	        this.summary = source["summary"];
	        this.last_error = source["last_error"];
	        this.generated_at = source["generated_at"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DoneInboxItem {
	    id: string;
	    task_id: string;
	    session_id: string;
	    agent: string;
	    project: string;
	    cwd?: string;
	    title?: string;
	    status: string;
	    summary?: string;
	    completed_at?: string;
	    reviewed?: boolean;
	    archived?: boolean;
	    pane_id?: number;
	    diff?: DiffRadar;
	    debrief?: Debrief;
	
	    static createFrom(source: any = {}) {
	        return new DoneInboxItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.task_id = source["task_id"];
	        this.session_id = source["session_id"];
	        this.agent = source["agent"];
	        this.project = source["project"];
	        this.cwd = source["cwd"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.summary = source["summary"];
	        this.completed_at = source["completed_at"];
	        this.reviewed = source["reviewed"];
	        this.archived = source["archived"];
	        this.pane_id = source["pane_id"];
	        this.diff = this.convertValues(source["diff"], DiffRadar);
	        this.debrief = this.convertValues(source["debrief"], Debrief);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Event {
	    at: string;
	    type: string;
	    text: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new Event(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.at = source["at"];
	        this.type = source["type"];
	        this.text = source["text"];
	        this.detail = source["detail"];
	    }
	}
	export class FlightEvent {
	    id: string;
	    parent_id?: string;
	    at?: string;
	    kind: string;
	    title: string;
	    detail?: string;
	    status?: string;
	    duration_millis?: number;
	    source?: string;
	
	    static createFrom(source: any = {}) {
	        return new FlightEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.parent_id = source["parent_id"];
	        this.at = source["at"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.detail = source["detail"];
	        this.status = source["status"];
	        this.duration_millis = source["duration_millis"];
	        this.source = source["source"];
	    }
	}
	export class Mission {
	    id: string;
	    name: string;
	    cwd?: string;
	    repo_root?: string;
	    status: string;
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
	
	    static createFrom(source: any = {}) {
	        return new Mission(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.cwd = source["cwd"];
	        this.repo_root = source["repo_root"];
	        this.status = source["status"];
	        this.task_ids = source["task_ids"];
	        this.attention = source["attention"];
	        this.working = source["working"];
	        this.drift = source["drift"];
	        this.blocked = source["blocked"];
	        this.idle = source["idle"];
	        this.completed = source["completed"];
	        this.changed_files = source["changed_files"];
	        this.last_event_at = source["last_event_at"];
	        this.summary = source["summary"];
	    }
	}
	
	
	export class TraceSpan {
	    id: string;
	    parent_id?: string;
	    kind: string;
	    name: string;
	    status: string;
	    started_at?: string;
	    ended_at?: string;
	    duration_millis?: number;
	    summary?: string;
	    detail?: string;
	    exit_code?: number;
	    children?: TraceSpan[];
	
	    static createFrom(source: any = {}) {
	        return new TraceSpan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.parent_id = source["parent_id"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.started_at = source["started_at"];
	        this.ended_at = source["ended_at"];
	        this.duration_millis = source["duration_millis"];
	        this.summary = source["summary"];
	        this.detail = source["detail"];
	        this.exit_code = source["exit_code"];
	        this.children = this.convertValues(source["children"], TraceSpan);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Session {
	    id: string;
	    agent: string;
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
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.agent = source["agent"];
	        this.cwd = source["cwd"];
	        this.log_path = source["log_path"];
	        this.title = source["title"];
	        this.last_prompt = source["last_prompt"];
	        this.created_at = source["created_at"];
	        this.last_event_at = source["last_event_at"];
	        this.pid = source["pid"];
	        this.process_uuid = source["process_uuid"];
	        this.internal = source["internal"];
	        this.internal_reason = source["internal_reason"];
	        this.events = this.convertValues(source["events"], Event);
	        this.trace = this.convertValues(source["trace"], TraceSpan);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TraceRef {
	    id: string;
	    kind: string;
	    name: string;
	    started_at?: string;
	    summary?: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new TraceRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.started_at = source["started_at"];
	        this.summary = source["summary"];
	        this.detail = source["detail"];
	    }
	}
	export class StatusExplanation {
	    status: string;
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
	
	    static createFrom(source: any = {}) {
	        return new StatusExplanation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.rule = source["rule"];
	        this.rule_severity = source["rule_severity"];
	        this.reason = source["reason"];
	        this.why_not_working = source["why_not_working"];
	        this.why_not_idle = source["why_not_idle"];
	        this.latest_assistant_at = source["latest_assistant_at"];
	        this.last_event_at = source["last_event_at"];
	        this.active_trace = this.convertValues(source["active_trace"], TraceRef);
	        this.background_run_traces = this.convertValues(source["background_run_traces"], TraceRef);
	        this.attention_event = this.convertValues(source["attention_event"], Event);
	        this.evidence = source["evidence"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Task {
	    id: string;
	    session: Session;
	    binding: Binding;
	    status: string;
	    status_explanation?: StatusExplanation;
	    attention_reason?: string;
	    summary?: string;
	    summary_at?: string;
	    diff?: DiffRadar;
	    flight?: FlightEvent[];
	    debrief?: Debrief;
	    archived?: boolean;
	    ignored?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.session = this.convertValues(source["session"], Session);
	        this.binding = this.convertValues(source["binding"], Binding);
	        this.status = source["status"];
	        this.status_explanation = this.convertValues(source["status_explanation"], StatusExplanation);
	        this.attention_reason = source["attention_reason"];
	        this.summary = source["summary"];
	        this.summary_at = source["summary_at"];
	        this.diff = this.convertValues(source["diff"], DiffRadar);
	        this.flight = this.convertValues(source["flight"], FlightEvent);
	        this.debrief = this.convertValues(source["debrief"], Debrief);
	        this.archived = source["archived"];
	        this.ignored = source["ignored"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Snapshot {
	    built_at: string;
	    tasks: Task[];
	    panes: Pane[];
	    processes: Process[];
	    missions?: Mission[];
	    done_inbox?: DoneInboxItem[];
	    errors?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.built_at = source["built_at"];
	        this.tasks = this.convertValues(source["tasks"], Task);
	        this.panes = this.convertValues(source["panes"], Pane);
	        this.processes = this.convertValues(source["processes"], Process);
	        this.missions = this.convertValues(source["missions"], Mission);
	        this.done_inbox = this.convertValues(source["done_inbox"], DoneInboxItem);
	        this.errors = source["errors"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	

}

export namespace config {
	
	export class AttentionRule {
	    id: string;
	    name: string;
	    enabled: boolean;
	    pattern: string;
	    severity: string;
	    scope?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new AttentionRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.pattern = source["pattern"];
	        this.severity = source["severity"];
	        this.scope = source["scope"];
	        this.message = source["message"];
	    }
	}
	export class Settings {
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
	    binding_mode: string;
	    notify_attention: boolean;
	    notify_completed: boolean;
	    notify_stuck: boolean;
	    notification_mode: string;
	    quiet_hours_enabled: boolean;
	    quiet_hours_start: string;
	    quiet_hours_end: string;
	    attention_rules?: AttentionRule[];
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cockpit_owner = source["cockpit_owner"];
	        this.codex_home = source["codex_home"];
	        this.claude_home = source["claude_home"];
	        this.codex_bin = source["codex_bin"];
	        this.wezterm_bin = source["wezterm_bin"];
	        this.recent_window_hours = source["recent_window_hours"];
	        this.idle_after_seconds = source["idle_after_seconds"];
	        this.stuck_after_seconds = source["stuck_after_seconds"];
	        this.summary_interval_seconds = source["summary_interval_seconds"];
	        this.max_sessions = source["max_sessions"];
	        this.show_unbound = source["show_unbound"];
	        this.enable_llm_summary = source["enable_llm_summary"];
	        this.binding_mode = source["binding_mode"];
	        this.notify_attention = source["notify_attention"];
	        this.notify_completed = source["notify_completed"];
	        this.notify_stuck = source["notify_stuck"];
	        this.notification_mode = source["notification_mode"];
	        this.quiet_hours_enabled = source["quiet_hours_enabled"];
	        this.quiet_hours_start = source["quiet_hours_start"];
	        this.quiet_hours_end = source["quiet_hours_end"];
	        this.attention_rules = this.convertValues(source["attention_rules"], AttentionRule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {

	export class ShareServerInfo {
	    url: string;
	    listen_addr: string;
	    readonly: boolean;

	    static createFrom(source: any = {}) {
	        return new ShareServerInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.listen_addr = source["listen_addr"];
	        this.readonly = source["readonly"];
	    }
	}

}
