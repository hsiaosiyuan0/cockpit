# Cockpit Product Spec

## Positioning

Cockpit is a local agent flight deck for Claude Code CLI and Codex CLI sessions.

The product does not replace the original terminal workflow. Users still discuss and confirm work inside Claude/Codex in WezTerm. Cockpit watches local logs, process state, WezTerm panes, git state, and optional LLM summaries so the user can stop reading noisy stdout during execution.

The core promise:

- Know which agents need attention.
- Know which agents are still working.
- Know what changed without watching stdout.
- Jump back to the exact WezTerm pane when human input is needed.

## Product Principles

- Observe, do not proxy. Cockpit should not become another chat client.
- Prefer status over transcript. Full logs are secondary; the default view should answer "what is happening now?"
- Keep agent internals isolated. Cockpit's own headless Codex runs must never pollute monitored sessions.
- Make uncertainty visible. Binding confidence, status logic, and error reasons should be explainable.
- Optimize for glancing. The dashboard should work like an operations panel, not a document reader.
- Preserve escape hatches. Users can always open the original pane, hide noisy sessions, refresh summary, or inspect trace.

## Current Baseline

The current app already covers the first useful loop:

- Discover recent Claude/Codex sessions from local logs.
- Bind sessions to WezTerm panes using pid, tty, cwd, and process evidence.
- Display mission board, selected task details, trace, and status logic.
- Detect idle, working, waiting, attention, and error-like states from logs and process context.
- Generate summaries through isolated headless Codex runs.
- Filter Cockpit internal runs out of the dashboard.
- Notify on completed, attention, and stuck categories.
- Open the bound WezTerm pane from the desktop app.
- On macOS, focus WezTerm after pane activation through DarwinKit/AppKit.

## Core User Flow

1. User starts a Claude/Codex session in WezTerm.
2. User discusses and confirms the task with the agent.
3. User leaves the terminal alone.
4. Cockpit automatically discovers and binds the session.
5. Cockpit shows current status, summary, trace, git activity, and attention signals.
6. When work completes or needs human input, Cockpit notifies the user.
7. User clicks the task and Cockpit opens the original WezTerm pane.
8. User reviews final changes, debrief, and any remaining risks.

## Feature Roadmap

## Implementation Progress

This section is updated while the prototype is being implemented.

- Flight Recorder: MVP implemented. Snapshot task model includes derived `flight` events from logs, trace spans, status transitions, and git radar; frontend selected task panel shows the recent flight recorder.
- Done Inbox: MVP implemented. Persistent `done.json` store, snapshot `done_inbox`, toolbar badge, inbox dialog, review, and archive actions are wired.
- Diff Radar: MVP implemented. Backend git scanner adds per-task dirty file summary; frontend selected task panel shows changed files and counts.
- Stuck Detector: MVP implemented. Status model includes `drift` and `blocked`; long-running traces and repeated failures can escalate with status logic evidence.
- Mission Groups: MVP implemented. Snapshot mission grouping is built from repo/cwd and frontend board can toggle between sessions and missions.
- One-click Debrief: MVP implemented. Manual debrief button runs isolated headless Codex and persists debrief output.
- Attention Rules: MVP implemented. Settings include configurable rules with enabled/severity/pattern controls and backend status logic consumes them.
- Command Palette: MVP implemented. `Cmd+K` opens search across sessions and missions with select/open/debrief actions.
- Quiet Hours / Focus Mode: MVP implemented. Settings and notification gating support normal/focus/silent plus quiet-hour suppression for completed notifications.
- Cockpit Theme Details: MVP implemented. Toolbar badge, mission cards, diff radar chips, flight recorder, command palette, and status colors add flight-deck detail.
- Mission Radar: MVP implemented. A radar band plots mission groups as status-colored targets and supports click-to-select.
- Master Caution Panel: MVP implemented. The top band surfaces the highest-severity attention/blocked/drift/waiting tasks.
- Flight Strip Rows: MVP implemented. Session rows now use callsigns plus pane/status/age/diff heat strip structure.
- Diff Heat Strip: MVP implemented. Task rows and ambient mode show compact changed-file type strips for code/test/lock/add/delete signals.
- Trace Minimap: MVP implemented. Expanded trace view includes a vertical span density map for tool/assistant/subagent/failure activity.
- Replay Mode: MVP implemented. Expanded Flight Recorder includes a replay slider and active event highlight.
- Resizable Split Panes: MVP implemented. Mission board/detail split can be dragged and is persisted in browser local storage.
- Ambient Mode: MVP implemented. Toolbar ambient mode opens a monitor-style view with mission radar, counts, and active flight strips.
- Command HUD: MVP implemented. `Cmd+K` is grouped into actions, missions, and sessions.
- Launch Feedback: MVP implemented. Opening a pane gives the selected row/detail a short target-lock animation.

### 1. Flight Recorder

Flight Recorder turns every agent session into a compact timeline.

Goal:

- Preserve the important sequence of events without requiring the user to read raw stdout.

Default view:

- Show a condensed timeline: user prompt, assistant response, tool use, subagent, error, status transition, done.
- Hide low-signal spans by default.
- Allow expanding a span for raw details when needed.

Timeline events:

- `session_started`
- `user_message`
- `assistant_message`
- `tool_started`
- `tool_completed`
- `tool_failed`
- `subagent_started`
- `subagent_completed`
- `status_changed`
- `attention_detected`
- `summary_generated`
- `session_completed`

Useful details:

- Timestamp
- Duration
- Actor
- Tool name
- Command preview
- Exit status
- Files touched when available
- Related status rule

Acceptance criteria:

- The selected task panel can show recent active spans by default.
- Fullscreen trace can show timestamps and more history.
- Users can distinguish current activity from old history at a glance.

### 2. Done Inbox

Done Inbox is the review queue for completed work.

Goal:

- Prevent completed tasks from disappearing into notifications.
- Give the user a daily review surface.

Inbox item fields:

- Task title
- Agent type
- Project/cwd
- Completion time
- Final status
- Final summary
- Files changed
- Tests run
- Errors seen
- Open pane action
- Archive action

States:

- `new`
- `reviewed`
- `archived`

UX:

- A small count in the top bar: `Done 3`.
- Clicking opens a review list.
- Items are grouped by project and day.
- The default action opens the original pane.
- Secondary action opens the diff radar for the task.

Acceptance criteria:

- A completed task remains visible until reviewed or archived.
- Completed notifications link to the Done Inbox item.
- The app can answer "what did my agents finish since I last checked?"

### 3. Diff Radar

Diff Radar shows what the agent actually changed in the working tree.

Goal:

- Replace noisy execution logs with concrete repo impact.

Data sources:

- `git status --porcelain=v1`
- `git diff --stat`
- `git diff --name-status`
- Optional focused diff summary generated by isolated Codex.

View:

- Added, modified, deleted, renamed files.
- Test files highlighted separately.
- Config and lockfile changes highlighted.
- Large churn called out.
- Dirty state grouped by project/cwd.

Status impact:

- If an agent is marked idle and the repo has new changes, surface "ready to review".
- If a working agent repeatedly changes the same files, show "active churn".
- If a merge conflict appears, escalate to attention.

Acceptance criteria:

- Selected task shows a compact changed-files summary.
- Full diff view can be opened from the task panel.
- Diff Radar does not run outside trusted local cwd paths.

### 4. Stuck Detector

Stuck Detector identifies when an agent is not making useful progress.

Goal:

- Detect bad waiting states earlier than a simple idle timeout.

Signals:

- Tool running longer than threshold.
- Same failing command repeated.
- Assistant asks for a decision.
- Permission denied.
- Authentication failure.
- Merge conflict.
- Test failure with no follow-up activity.
- No new log event for too long while process still exists.
- Subagent running beyond threshold.
- Repeated summary with no meaningful progress.

Status labels:

- `ATTN`: human decision needed.
- `BLOCKED`: agent cannot proceed without external fix.
- `DRIFT`: activity exists but progress looks suspicious.
- `IDLE`: response appears finished.
- `WORKING`: active tool/subagent/process activity.

Status logic panel:

- Primary reason.
- Last assistant timestamp.
- Active tool/subagent if any.
- Background process if any.
- Why not working.
- Why not idle.

Acceptance criteria:

- Status logic explains every non-obvious classification.
- Long tool/subagent runs keep a session working, not idle.
- Assistant text asking a question can escalate to attention.

### 5. Mission Groups

Mission Groups combine multiple sessions around the same project or objective.

Goal:

- Let the dashboard show work at the project/mission level instead of only session rows.

Grouping sources:

- cwd/project root
- git repo
- task title similarity
- manual mission assignment
- branch name

Mission summary:

- Active sessions
- Done sessions
- Attention sessions
- Changed files
- Latest summary
- Open all related panes

UX:

- Mission Board can switch between `Sessions` and `Missions`.
- A mission can be expanded to show child sessions.
- Mission status is the highest-severity status among child sessions.

Acceptance criteria:

- Users can see "release-web has 3 agents, 1 needs attention, 2 idle".
- Manual grouping exists for cases where cwd is not enough.

### 6. One-click Debrief

One-click Debrief creates a focused progress report on demand.

Goal:

- Ask "what happened and what should I do next?" without manually reading logs and diffs.

Inputs:

- Recent session log slices.
- Trace events.
- Status logic.
- Git diff summary.
- Test command evidence.
- Existing summary cache.

Output:

- Completed work.
- Current state.
- Files changed.
- Tests run and result.
- Risks.
- Open questions.
- Suggested next action.

Isolation requirements:

- Debrief runs through Cockpit's internal headless Codex process.
- Internal runs are marked with `COCKPIT_INTERNAL=1`.
- Internal cwd stays under Cockpit internal run directory.
- Internal sessions are filtered from discovery and summaries.

Acceptance criteria:

- Debrief is manual by default.
- Generated text is saved with the task.
- User can refresh it without creating visible phantom sessions.

### 7. Attention Rules

Attention Rules make status escalation configurable.

Goal:

- Let users define project-specific or personal attention patterns without recompiling.

Rule types:

- Text contains.
- Regex match.
- Tool failure.
- Exit code.
- Long-running command.
- File pattern changed.
- Session inactive after assistant question.

Rule fields:

- Name
- Enabled
- Scope: global, agent type, project, cwd
- Pattern or signal
- Severity: attention, blocked, drift
- Message

Default rules:

- Permission denied.
- Authentication required.
- Merge conflict.
- Test failed.
- Agent asks for a decision.
- Tool exceeds threshold.

UX:

- Settings panel lists rules.
- Status Logic links the active rule name.
- Users can disable noisy rules.

Acceptance criteria:

- Attention state shows the matching rule.
- Rule changes apply without restarting the app.

### 8. Command Palette

Command Palette gives fast keyboard access.

Goal:

- Make the desktop app efficient when there are many sessions.

Shortcut:

- `Cmd+K`

Commands:

- Search session.
- Search project.
- Open pane.
- Bind pane.
- Hide/unhide session.
- Refresh summary.
- Generate debrief.
- Open Done Inbox.
- Toggle settings.
- Toggle trace fullscreen.

Search fields:

- Task title
- Agent type
- cwd
- session id
- pane id
- summary text

Acceptance criteria:

- Common actions are reachable without using the mouse.
- Search remains fast with dozens of sessions.

### 9. Quiet Hours and Focus Mode

Quiet Hours reduce notification noise.

Goal:

- Let Cockpit be useful without interrupting deep work.

Modes:

- `Normal`: notify attention, completed, stuck.
- `Focus`: notify only attention/blocking issues.
- `Quiet Hours`: scheduled suppression.
- `Silent Project`: per-project notification mute.

Notification behavior:

- Suppressed notifications still appear in the app.
- Done Inbox count still increments.
- Attention count still increments.

Acceptance criteria:

- User can silence non-critical notifications.
- Critical attention signals remain visible in the dashboard.

### 10. Cockpit Visual System

The visual system should feel like an agent flight deck without sacrificing readability.

Goal:

- Make the app feel purposeful and inspectable, not like a generic table UI.

Design direction:

- Dense operational layout.
- Dark instrument-panel base.
- Clear severity colors.
- Monospace data where useful.
- Subtle grid and panel borders.
- Activity pulses only where they communicate live state.
- Fullscreen trace/status panels for deep inspection.

Avoid:

- Decorative animations that distract from status.
- Large marketing-style hero sections.
- Too much transcript by default.
- One-note color palettes.

Useful visual elements:

- Mission counters.
- Signal/confidence indicators.
- Active tool pulse.
- Done Inbox badge.
- Diff Radar changed-file chips.
- Status reason strip.
- Trace span density bars.

Acceptance criteria:

- A 14-inch laptop screen shows the mission board and selected task without horizontal overflow.
- Fullscreen panels avoid the macOS traffic-light area.
- Trace and Status Logic remain readable at default window size.

## Suggested Implementation Order

### Phase 1: Operational Clarity

Ship first because it strengthens the core promise.

- Diff Radar compact view.
- Done Inbox.
- Stuck Detector rule refinements.
- Status Logic explanations for new states.

### Phase 2: Review and Replay

Ship after the status loop is reliable.

- Flight Recorder timeline model.
- Fullscreen trace history with timestamps.
- One-click Debrief.
- Debrief persistence.

### Phase 3: Scale and Control

Ship when many sessions are common.

- Mission Groups.
- Command Palette.
- Attention Rules UI.
- Quiet Hours and Focus Mode.

### Phase 4: Polish

Ship continuously, but only after behavior is solid.

- Cockpit visual refinements.
- Activity pulse details.
- Better empty states.
- Better responsive sizing for 14-inch laptops.

## Open Product Decisions

- Should Done Inbox be task-based, session-based, or mission-based?
- Should Diff Radar read only git repos, or also support non-git directories?
- Should Mission Groups be automatic first, manual first, or hybrid?
- How much raw trace should be retained locally?
- Should attention rules be stored globally only, or allow per-project config files?
- Should One-click Debrief ever run automatically, or stay manual to avoid noise and cost?

## Non-goals for Now

- Do not run Claude/Codex through Cockpit as a proxy terminal.
- Do not implement a full chat UI.
- Do not sync sessions to a cloud service.
- Do not mutate user repos except for explicit Cockpit config/state.
- Do not auto-fix agent failures without user confirmation.

## Success Criteria

Cockpit is working when the user can leave several Claude/Codex sessions running, glance at the dashboard later, and answer:

- Which tasks need me?
- Which tasks are truly working?
- Which tasks are done?
- What changed?
- What failed?
- Where do I jump back in?
