package engine

import (
	"context"
	"sort"
	"strings"
	"time"

	"cockpit/internal/app"
	"cockpit/internal/binder"
	"cockpit/internal/config"
	"cockpit/internal/discovery"
	"cockpit/internal/flight"
	"cockpit/internal/gitradar"
	"cockpit/internal/process"
	"cockpit/internal/state"
	"cockpit/internal/wezterm"
)

type SnapshotOptions struct {
	FallbackPanes     []app.Pane
	FallbackPanesUsed *bool
}

func BuildSnapshot(ctx context.Context, cfg config.Config, store *app.Store) (app.Snapshot, error) {
	return BuildSnapshotWithOptions(ctx, cfg, store, SnapshotOptions{})
}

func BuildSnapshotWithOptions(ctx context.Context, cfg config.Config, store *app.Store, options SnapshotOptions) (app.Snapshot, error) {
	var snap app.Snapshot
	snap.BuiltAt = time.Now()

	processes, err := process.List(ctx)
	if err != nil {
		snap.Errors = append(snap.Errors, "process list: "+err.Error())
	}
	panes, err := wezterm.ListPanes(ctx, cfg.WeztermBin)
	if err != nil {
		if len(options.FallbackPanes) > 0 {
			panes = clonePanes(options.FallbackPanes)
			if options.FallbackPanesUsed != nil {
				*options.FallbackPanesUsed = true
			}
		} else {
			snap.Errors = append(snap.Errors, "wezterm panes: "+err.Error())
		}
	}
	sessions, err := discovery.DiscoverSessions(ctx, cfg)
	if err != nil {
		snap.Errors = append(snap.Errors, "discover sessions: "+err.Error())
	}

	userState := store.LoadUserState()
	bindings := binder.Bind(sessions, processes, panes)
	if cfg.BindingMode == "manual" {
		clearAutoBindings(bindings)
	}
	applyManualBindings(bindings, sessions, panes, userState.ManualBindings)
	diffByCWD := map[string]app.DiffRadar{}
	tasks := make([]app.Task, 0, len(sessions))
	for _, session := range sessions {
		taskID := app.TaskID(session)
		task := app.Task{
			ID:       taskID,
			Session:  session,
			Binding:  bindings[taskID],
			Archived: userState.Archived[taskID],
			Ignored:  userState.Ignored[taskID],
		}
		if session.CWD != "" {
			if radar, ok := diffByCWD[session.CWD]; ok {
				task.Diff = radar
			} else {
				task.Diff = gitradar.Scan(ctx, session.CWD)
				diffByCWD[session.CWD] = task.Diff
			}
		}
		task.StatusExplain = state.ExplainWithOptions(task, state.Options{
			IdleAfter:      cfg.IdleAfter,
			StuckAfter:     cfg.StuckAfter,
			AttentionRules: cfg.AttentionRules,
		})
		task.Status = task.StatusExplain.Status
		if task.Status == app.StatusNeedsAttention || task.Status == app.StatusBlocked || task.Status == app.StatusDrift {
			task.AttentionReason = task.StatusExplain.Reason
		}
		if cache, ok := store.LoadSummary(taskID); ok {
			task.Summary = cache.Summary
			task.SummaryAt = cache.GeneratedAt
		}
		if task.Summary == "" {
			task.Summary = state.RuleSummary(task)
		}
		if debrief, ok := store.LoadDebrief(taskID); ok {
			task.Debrief = &debrief
		}
		task.Flight = flight.Build(task)
		tasks = append(tasks, task)
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Status != tasks[j].Status {
			return app.StatusRank(tasks[i].Status) < app.StatusRank(tasks[j].Status)
		}
		return tasks[i].Session.LastEventAt.After(tasks[j].Session.LastEventAt)
	})
	snap.Tasks = tasks
	snap.Panes = panes
	snap.Processes = processes
	snap.Missions = buildMissions(tasks, userState.MissionNames)
	if done, err := store.SyncDoneInbox(tasks); err != nil {
		snap.Errors = append(snap.Errors, err.Error())
	} else {
		snap.DoneInbox = done
	}
	return snap, nil
}

func clonePanes(panes []app.Pane) []app.Pane {
	if len(panes) == 0 {
		return nil
	}
	cloned := make([]app.Pane, len(panes))
	copy(cloned, panes)
	return cloned
}

func buildMissions(tasks []app.Task, customNames map[string]string) []app.Mission {
	byKey := map[string]*app.Mission{}
	for _, task := range tasks {
		if task.Session.Internal || task.Archived || task.Ignored {
			continue
		}
		key := missionKey(task)
		mission := byKey[key]
		if mission == nil {
			name := task.RepoName()
			if task.Diff.RepoRoot != "" {
				name = repoName(task.Diff.RepoRoot)
			}
			defaultName := name
			customName := strings.TrimSpace(customNames[key])
			if customName != "" {
				name = customName
			}
			mission = &app.Mission{
				ID:          key,
				Name:        name,
				DefaultName: defaultName,
				Renamed:     customName != "",
				CWD:         task.Session.CWD,
				RepoRoot:    task.Diff.RepoRoot,
				Status:      task.Status,
				Summary:     task.Summary,
			}
			byKey[key] = mission
		}
		mission.TaskIDs = append(mission.TaskIDs, task.ID)
		if app.StatusRank(task.Status) < app.StatusRank(mission.Status) {
			mission.Status = task.Status
			mission.Summary = task.Summary
		}
		if task.Session.LastEventAt.After(mission.LastEventAt) {
			mission.LastEventAt = task.Session.LastEventAt
		}
		if task.Diff.Dirty {
			mission.ChangedFiles += len(task.Diff.Files)
		}
		switch task.Status {
		case app.StatusNeedsAttention, app.StatusWaiting:
			mission.Attention++
		case app.StatusWorking:
			mission.Working++
		case app.StatusDrift:
			mission.Drift++
		case app.StatusBlocked:
			mission.Blocked++
		case app.StatusIdle:
			mission.Idle++
		case app.StatusCompleted:
			mission.Completed++
		}
	}
	missions := make([]app.Mission, 0, len(byKey))
	for _, mission := range byKey {
		missions = append(missions, *mission)
	}
	sort.SliceStable(missions, func(i, j int) bool {
		if missions[i].Status != missions[j].Status {
			return app.StatusRank(missions[i].Status) < app.StatusRank(missions[j].Status)
		}
		return missions[i].LastEventAt.After(missions[j].LastEventAt)
	})
	return missions
}

func missionKey(task app.Task) string {
	if task.Diff.RepoRoot != "" {
		return "repo:" + task.Diff.RepoRoot
	}
	if task.Session.CWD != "" {
		return "cwd:" + task.Session.CWD
	}
	return "agent:" + string(task.Session.Agent)
}

func repoName(path string) string {
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "unknown"
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func clearAutoBindings(bindings map[string]app.Binding) {
	for taskID, binding := range bindings {
		if binding.Pane == nil {
			continue
		}
		binding.Pane = nil
		binding.Process = nil
		binding.Confidence = 0
		binding.Reasons = []string{"auto binding disabled"}
		bindings[taskID] = binding
	}
}

func applyManualBindings(bindings map[string]app.Binding, sessions []app.Session, panes []app.Pane, manual map[string]int) {
	if len(manual) == 0 {
		return
	}
	panesByID := map[int]app.Pane{}
	for _, pane := range panes {
		panesByID[pane.PaneID] = pane
	}
	for _, session := range sessions {
		taskID := app.TaskID(session)
		paneID, ok := manual[taskID]
		if !ok {
			continue
		}
		pane, ok := panesByID[paneID]
		if !ok {
			continue
		}
		bindings[taskID] = app.Binding{
			Pane:       &pane,
			Confidence: 999,
			Reasons:    []string{"manual attach"},
		}
	}
}
