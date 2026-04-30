# Cockpit

Cockpit is a local flight deck for Claude Code and Codex CLI sessions.

The idea is simple: I still talk to agents in the terminal, but I do not want to babysit their stdout. Cockpit watches the local session logs, WezTerm panes, process state, and git working trees, then gives me a dashboard that answers the useful questions:

- Which agents need my attention?
- Which ones are still working?
- Which ones are idle or done?
- What files did they actually change?
- Which WezTerm pane should I jump back to?

Cockpit is not a chat client and does not proxy Claude or Codex. The original terminal session remains the source of truth.

## What It Does

- Discovers recent Claude Code and Codex CLI sessions from local logs.
- Binds sessions to WezTerm panes using pid, tty, cwd, and process evidence.
- Shows status categories like attention, blocked, drift, working, idle, and done.
- Builds a trace view from tool use, assistant replies, and subagent spans.
- Shows a Diff Radar for working tree changes.
- Keeps completed work in a Done Inbox so it can be reviewed later.
- Generates summaries and debriefs with an isolated headless Codex run.
- Opens the original WezTerm pane when I need to respond.
- On macOS, focuses WezTerm after pane activation.

## Current Shape

This is still a prototype, but the main loop is already there. The desktop app is the primary interface. The old TUI code is still in the repository because it helped prove out discovery, binding, and status logic.

The UI is intentionally cockpit-like: mission board, radar, master caution, diff radar, trace, flight recorder, and status logic. The goal is fast situational awareness, not another wall of logs.

## Requirements

- Go
- Node.js and npm
- Wails v2
- WezTerm
- Claude Code CLI and/or Codex CLI if you want live data

The browser frontend can run with mock data, so it is still useful for UI work without the native app.

## Run The Desktop App

```sh
wails dev
```

Build a packaged app:

```sh
wails build
open build/bin/cockpit.app
```

If `wails` is not on your `PATH`, use the local Wails binary you installed. In my environment I often run:

```sh
/tmp/cockpit-bin/wails build
```

## Work On The Frontend

```sh
cd frontend
npm install
npm run dev
```

When the frontend is opened outside Wails, it falls back to mock data. Inside the desktop app it calls the Go backend through Wails bindings.

## CLI And TUI

```sh
go run ./cmd/cockpit
go run ./cmd/cockpit demo
go run ./cmd/cockpit inspect
go run ./cmd/cockpit doctor
```

Manual pane binding is available when automatic binding gets it wrong:

```sh
cockpit attach <task-or-session-prefix>
cockpit attach --detach <task-or-session-prefix>
```

## Settings

The desktop app has a settings panel for:

- Claude, Codex, and WezTerm paths
- idle and stuck thresholds
- automatic vs manual binding
- unbound session visibility
- LLM summaries
- notification mode and quiet hours
- attention rules

Settings are stored under `~/.local/state/cockpit` by default.

## Internal Codex Runs

Summaries and debriefs are generated with isolated headless Codex runs. Cockpit marks those runs with internal environment variables and filters them out of the dashboard, so its own analysis process does not pollute the sessions it is watching.

## Development Checks

```sh
go test ./...
cd frontend && npm run build
```

For a full desktop build:

```sh
wails build
```

## Notes

This app is deliberately local-first. It reads local logs, local git state, local process state, and local WezTerm state. That keeps the workflow close to the terminal sessions it is trying to make easier to monitor.
