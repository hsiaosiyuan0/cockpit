package app

import "time"

func DemoSnapshot() Snapshot {
	now := time.Now()
	tasks := DemoTasksAt(now)
	return Snapshot{
		BuiltAt:   now,
		Tasks:     tasks,
		Missions:  DemoMissionsAt(now),
		DoneInbox: DemoDoneInboxAt(now),
		Panes: []Pane{
			{PaneID: 7, WindowID: 1, TabID: 3, CWD: "/Users/hsy/dev/frontend", TTYName: "ttys007", IsActive: false, Workspace: "main", TabTitle: "frontend"},
			{PaneID: 2, WindowID: 1, TabID: 1, CWD: "/Users/hsy/dev/cockpit", TTYName: "ttys002", IsActive: true, Workspace: "main", TabTitle: "cockpit"},
			{PaneID: 0, WindowID: 1, TabID: 0, CWD: "/Users/hsy/work/kb-workspace", TTYName: "ttys000", IsActive: false, Workspace: "main", TabTitle: "music"},
			{PaneID: 4, WindowID: 1, TabID: 2, CWD: "/Users/hsy/work/kb-workspace", TTYName: "ttys004", IsActive: false, Workspace: "main", TabTitle: "deploy"},
		},
	}
}

func DemoTasks() []Task {
	return DemoTasksAt(time.Now())
}

func DemoTasksAt(now time.Time) []Task {
	return []Task{
		{
			ID: "codex:019de0b2-71d0-demo",
			Session: Session{
				ID:          "019de0b2-71d0-demo",
				Agent:       AgentCodex,
				CWD:         "/Users/hsy/dev/frontend",
				Title:       "Route guard refactor",
				LastEventAt: now.Add(-2 * time.Minute),
				Events: []Event{
					{At: now.Add(-7 * time.Minute), Type: EventTool, Text: "edited src/router/guards.ts"},
					{At: now.Add(-6 * time.Minute), Type: EventTool, Text: "ran pnpm test router.guard.spec.ts"},
					{At: now.Add(-5 * time.Minute), Type: EventResult, Text: "1 failed, 12 passed"},
					{At: now.Add(-2 * time.Minute), Type: EventAssistant, Text: "asking whether to preserve legacy redirect behavior"},
				},
				Trace: []TraceSpan{
					{
						ID:        "turn:1",
						Kind:      TraceTurn,
						Name:      "user turn",
						Status:    TraceSucceeded,
						StartedAt: now.Add(-7 * time.Minute),
						EndedAt:   now.Add(-2 * time.Minute),
						Summary:   "refactor route guard behavior",
						Children: []TraceSpan{
							{ID: "call:edit", ParentID: "turn:1", Kind: TraceTool, Name: "apply_patch", Status: TraceSucceeded, StartedAt: now.Add(-7 * time.Minute), EndedAt: now.Add(-6*time.Minute - 30*time.Second), Summary: "edited src/router/guards.ts"},
							{ID: "call:test", ParentID: "turn:1", Kind: TraceTool, Name: "exec_command", Status: TraceFailed, StartedAt: now.Add(-6 * time.Minute), EndedAt: now.Add(-5 * time.Minute), ExitCode: intPtr(1), Detail: "pnpm test router.guard.spec.ts", Summary: "1 failed, 12 passed"},
							{ID: "assistant:1", ParentID: "turn:1", Kind: TraceAssistant, Name: "assistant", Status: TraceSucceeded, StartedAt: now.Add(-2 * time.Minute), EndedAt: now.Add(-2 * time.Minute), Summary: "asking whether to preserve legacy redirect behavior"},
						},
					},
				},
			},
			Binding: Binding{
				Pane:       &Pane{PaneID: 7, WindowID: 1, TabID: 3, CWD: "/Users/hsy/dev/frontend", TTYName: "ttys007", Workspace: "main", TabTitle: "frontend"},
				Confidence: 94,
				Reasons:    []string{"pid tty match", "cwd match"},
			},
			Status:          StatusNeedsAttention,
			AttentionReason: "tests failed; agent is waiting for a decision",
			Summary:         "tests failed after changing route guards; agent is waiting for direction",
			Diff: DiffRadar{
				RepoRoot:    "/Users/hsy/dev/frontend",
				Branch:      "agent/route-guard",
				Dirty:       true,
				Modified:    3,
				Untracked:   1,
				Tests:       2,
				Summary:     "3 modified, 1 untracked",
				GeneratedAt: now.Add(-20 * time.Second),
				Files: []DiffFile{
					{Path: "src/router/guards.ts", Status: "modified", Additions: 42, Deletions: 18},
					{Path: "src/router/guards.spec.ts", Status: "modified", Additions: 11, Deletions: 2},
					{Path: "test/fixtures/legacy-redirect.json", Status: "untracked"},
				},
			},
			Flight: []FlightEvent{
				{ID: "f1", At: now.Add(-7 * time.Minute), Kind: "user", Title: "user prompt", Detail: "refactor route guard behavior", Source: "log"},
				{ID: "f2", At: now.Add(-6 * time.Minute), Kind: "tool", Title: "pnpm test", Detail: "router.guard.spec.ts", Status: "failed", Source: "trace"},
				{ID: "f3", At: now.Add(-2 * time.Minute), Kind: "status", Title: "needs_attention", Detail: "tests failed; decision needed", Status: "needs_attention", Source: "status"},
			},
		},
		{
			ID: "codex:019ddc4c-620e-demo",
			Session: Session{
				ID:          "019ddc4c-620e-demo",
				Agent:       AgentCodex,
				CWD:         "/Users/hsy/dev/cockpit",
				Title:       "Cockpit desktop shell",
				LastEventAt: now.Add(-2 * time.Minute),
				Events: []Event{
					{At: now.Add(-4 * time.Minute), Type: EventUser, Text: "use mock data to restore the cockpit TUI style"},
					{At: now.Add(-3 * time.Minute), Type: EventTool, Text: "patched internal/tui/model.go"},
					{At: now.Add(-2 * time.Minute), Type: EventAssistant, Text: "desktop shell is updated and waiting for the next instruction"},
				},
				Trace: []TraceSpan{
					{
						ID:        "turn:1",
						Kind:      TraceTurn,
						Name:      "user turn",
						Status:    TraceSucceeded,
						StartedAt: now.Add(-4 * time.Minute),
						EndedAt:   now.Add(-2 * time.Minute),
						Summary:   "build cockpit desktop app",
						Children: []TraceSpan{
							{ID: "call:inspect", ParentID: "turn:1", Kind: TraceTool, Name: "exec_command", Status: TraceSucceeded, StartedAt: now.Add(-3 * time.Minute), EndedAt: now.Add(-2*time.Minute - 20*time.Second), Detail: "rg --files"},
							{ID: "subagent:ui", ParentID: "turn:1", Kind: TraceSubagent, Name: "spawn_agent", Status: TraceSucceeded, StartedAt: now.Add(-90 * time.Second), EndedAt: now.Add(-70 * time.Second), Detail: "frontend polish worker"},
							{ID: "call:build", ParentID: "turn:1", Kind: TraceTool, Name: "exec_command", Status: TraceSucceeded, StartedAt: now.Add(-70 * time.Second), EndedAt: now.Add(-65 * time.Second), Detail: "wails build"},
							{ID: "assistant:1", ParentID: "turn:1", Kind: TraceAssistant, Name: "assistant", Status: TraceSucceeded, StartedAt: now.Add(-2 * time.Minute), EndedAt: now.Add(-2 * time.Minute), Summary: "desktop shell is updated and waiting for the next instruction"},
						},
					},
				},
			},
			Binding: Binding{
				Pane:       &Pane{PaneID: 2, WindowID: 1, TabID: 1, CWD: "/Users/hsy/dev/cockpit", TTYName: "ttys002", Workspace: "main", TabTitle: "cockpit"},
				Confidence: 999,
				Reasons:    []string{"manual attach"},
			},
			Status: StatusIdle,
			StatusExplain: StatusExplanation{
				Status:            StatusIdle,
				Rule:              "idle_timeout",
				Reason:            "agent response ended; idle for 2m0s",
				LatestAssistantAt: now.Add(-2 * time.Minute),
				LastEventAt:       now.Add(-2 * time.Minute),
				Evidence:          []string{"assistant response is older than settle window"},
			},
			Summary: "desktop shell is updated and waiting for the next instruction",
			Diff: DiffRadar{
				RepoRoot:    "/Users/hsy/dev/cockpit",
				Branch:      "main",
				Dirty:       true,
				Added:       2,
				Modified:    6,
				Tests:       1,
				Summary:     "2 added, 6 modified",
				GeneratedAt: now.Add(-15 * time.Second),
				Files: []DiffFile{
					{Path: "internal/gitradar/gitradar.go", Status: "added", Additions: 210},
					{Path: "frontend/src/App.tsx", Status: "modified", Additions: 180, Deletions: 12},
				},
			},
			Debrief: &Debrief{
				TaskID:      "codex:019ddc4c-620e-demo",
				SessionID:   "019ddc4c-620e-demo",
				GeneratedAt: now.Add(-1 * time.Minute),
				Text:        "## Completed\nDesktop shell and status panels are wired.\n\n## Current State\nIdle and ready for review.\n\n## Changed Files\nFrontend and Go snapshot code changed.\n\n## Tests\nMock build passed in demo mode.\n\n## Risks\nNative sessions may differ from demo data.\n\n## Next Action\nReview the generated panels.",
				InputHash:   "demo",
			},
		},
		{
			ID: "claude:788291c5-19ee-demo",
			Session: Session{
				ID:          "788291c5-19ee-demo",
				Agent:       AgentClaude,
				CWD:         "/Users/hsy/work/kb-workspace/music-know-vault",
				Title:       "Flyway startup check",
				LastEventAt: now.Add(-3 * time.Minute),
				Events: []Event{
					{At: now.Add(-9 * time.Minute), Type: EventTool, Text: "read curation server pom.xml"},
					{At: now.Add(-4 * time.Minute), Type: EventTool, Text: "ran mvn test for Flyway startup"},
					{At: now.Add(-3 * time.Minute), Type: EventAssistant, Text: "confirmed c3p0 exclusion keeps Boot 3 datasource detection clean"},
				},
			},
			Binding: Binding{
				Pane:       &Pane{PaneID: 0, WindowID: 1, TabID: 0, CWD: "/Users/hsy/work/kb-workspace", TTYName: "ttys000", Workspace: "main", TabTitle: "music"},
				Confidence: 88,
				Reasons:    []string{"pid tty match", "cwd related", "agent process on pane tty"},
			},
			Status: StatusIdle,
			StatusExplain: StatusExplanation{
				Status:            StatusIdle,
				Rule:              "idle_timeout",
				Reason:            "agent response ended; idle for 3m0s",
				LatestAssistantAt: now.Add(-3 * time.Minute),
				LastEventAt:       now.Add(-3 * time.Minute),
				Evidence:          []string{"assistant response is older than settle window"},
			},
			Summary: "confirmed Flyway startup behavior after excluding a legacy c3p0 dependency",
		},
		{
			ID: "claude:57665bc0-123e-demo",
			Session: Session{
				ID:          "57665bc0-123e-demo",
				Agent:       AgentClaude,
				CWD:         "/Users/hsy/work/kb-workspace",
				Title:       "Deployment notes",
				LastEventAt: now.Add(-14 * time.Minute),
				Events: []Event{
					{At: now.Add(-21 * time.Minute), Type: EventUser, Text: "check deployment notes"},
					{At: now.Add(-18 * time.Minute), Type: EventTool, Text: "read deployment README"},
					{At: now.Add(-14 * time.Minute), Type: EventSystem, Text: "no new output since"},
				},
			},
			Binding: Binding{
				Pane:       &Pane{PaneID: 4, WindowID: 1, TabID: 2, CWD: "/Users/hsy/work/kb-workspace", TTYName: "ttys004", Workspace: "main", TabTitle: "deploy"},
				Confidence: 71,
				Reasons:    []string{"cwd match", "agent process on pane tty"},
			},
			Status:  StatusIdle,
			Summary: "last activity was reading deployment notes; no new output since",
		},
		{
			ID: "codex:019dd2e0-6a8c-demo",
			Session: Session{
				ID:          "019dd2e0-6a8c-demo",
				Agent:       AgentCodex,
				CWD:         "/Users/hsy/dev/my-buddies",
				Title:       "Unbound session",
				LastEventAt: now.Add(-42 * time.Minute),
				Events: []Event{
					{At: now.Add(-46 * time.Minute), Type: EventSystem, Text: "session found in Codex logs"},
					{At: now.Add(-42 * time.Minute), Type: EventSystem, Text: "no reliable pane match"},
				},
			},
			Status:  StatusUnbound,
			Summary: "session found in logs, but no reliable pane match",
		},
	}
}

func DemoMissionsAt(now time.Time) []Mission {
	return []Mission{
		{
			ID:           "repo:/Users/hsy/dev/frontend",
			Name:         "frontend",
			RepoRoot:     "/Users/hsy/dev/frontend",
			Status:       StatusNeedsAttention,
			TaskIDs:      []string{"codex:019de0b2-71d0-demo"},
			Attention:    1,
			ChangedFiles: 3,
			LastEventAt:  now.Add(-2 * time.Minute),
			Summary:      "tests failed after changing route guards",
		},
		{
			ID:           "repo:/Users/hsy/dev/cockpit",
			Name:         "cockpit",
			RepoRoot:     "/Users/hsy/dev/cockpit",
			Status:       StatusIdle,
			TaskIDs:      []string{"codex:019ddc4c-620e-demo"},
			Idle:         1,
			ChangedFiles: 2,
			LastEventAt:  now.Add(-2 * time.Minute),
			Summary:      "desktop shell is updated and waiting for the next instruction",
		},
	}
}

func DemoDoneInboxAt(now time.Time) []DoneInboxItem {
	paneID := 2
	return []DoneInboxItem{
		{
			ID:          "codex_019done-demo",
			TaskID:      "codex:019ddc4c-620e-demo",
			SessionID:   "019ddc4c-620e-demo",
			Agent:       AgentCodex,
			Project:     "cockpit",
			Title:       "Cockpit desktop shell",
			Status:      StatusCompleted,
			Summary:     "desktop shell is updated and waiting for review",
			CompletedAt: now.Add(-9 * time.Minute),
			PaneID:      &paneID,
			Diff: DiffRadar{
				RepoRoot:  "/Users/hsy/dev/cockpit",
				Branch:    "main",
				Dirty:     true,
				Added:     2,
				Modified:  6,
				Tests:     1,
				Summary:   "2 added, 6 modified",
				Lockfiles: 0,
			},
		},
	}
}

func intPtr(value int) *int {
	return &value
}
