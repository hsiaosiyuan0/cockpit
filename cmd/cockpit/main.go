package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"cockpit/internal/app"
	"cockpit/internal/config"
	"cockpit/internal/engine"
	"cockpit/internal/summary"
	"cockpit/internal/wezterm"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cockpit:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "doctor":
		return runDoctor(cfg)
	case "inspect":
		return runInspect(cfg, args[1:])
	case "summarize":
		return runSummarize(cfg, args[1:])
	case "attach":
		return runAttach(cfg, args[1:])
	case "open":
		return runOpen(cfg, args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Print(`cockpit watches Claude Code and Codex CLI sessions.

Usage:
  cockpit                 show this help
  cockpit inspect         print discovered tasks and bindings
  cockpit inspect --json  print JSON
  cockpit doctor          check local observability dependencies
  cockpit summarize ID    force LLM summary for one task/session id
  cockpit attach [ID]     bind a task/session to the current WezTerm pane
  cockpit open PANE_ID    activate a WezTerm pane
`)
}

func runInspect(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	withSummary := fs.Bool("summary", false, "refresh LLM summaries before printing")
	noLLM := fs.Bool("no-llm", false, "do not run LLM summarization")
	recent := fs.Duration("recent", cfg.RecentWindow, "session recency window")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	cfg.RecentWindow = *recent
	store, err := app.NewStore(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	snap, err := engine.BuildSnapshot(ctx, cfg, store)
	if err != nil {
		return err
	}
	if *withSummary && !*noLLM {
		sum := summary.NewCodexSummarizer(cfg, store)
		for i := range snap.Tasks {
			if snap.Tasks[i].Session.Internal || snap.Tasks[i].Archived || snap.Tasks[i].Ignored {
				continue
			}
			if err := sum.Refresh(ctx, &snap.Tasks[i]); err != nil {
				snap.Tasks[i].Summary = "summary error: " + err.Error()
			}
		}
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(snap.Tasks)
	}
	printTasks(snap.Tasks)
	return nil
}

func printTasks(tasks []app.Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Status != tasks[j].Status {
			return app.StatusRank(tasks[i].Status) < app.StatusRank(tasks[j].Status)
		}
		return tasks[i].Session.LastEventAt.After(tasks[j].Session.LastEventAt)
	})
	for _, task := range tasks {
		if task.Archived || task.Ignored || task.Session.Internal {
			continue
		}
		binding := "unbound"
		if task.Binding.Pane != nil {
			binding = fmt.Sprintf("pane=%d bound", task.Binding.Pane.PaneID)
			if hasReason(task.Binding.Reasons, "manual attach") {
				binding = fmt.Sprintf("pane=%d manual", task.Binding.Pane.PaneID)
			}
		}
		fmt.Printf("%-15s %-6s %-16s %-11s %s\n", task.IDShort(), task.Session.Agent, task.RepoName(), task.Status, binding)
		if task.Summary != "" {
			fmt.Printf("  summary: %s\n", task.Summary)
		}
		if task.AttentionReason != "" {
			fmt.Printf("  attention: %s\n", task.AttentionReason)
		}
		if len(task.Binding.Reasons) > 0 {
			fmt.Printf("  binding: %s\n", strings.Join(task.Binding.Reasons, ", "))
		}
		if task.Session.CWD != "" {
			fmt.Printf("  cwd: %s\n", task.Session.CWD)
		}
	}
}

func hasReason(reasons []string, reason string) bool {
	for _, candidate := range reasons {
		if candidate == reason {
			return true
		}
	}
	return false
}

func runSummarize(cfg config.Config, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: cockpit summarize TASK_OR_SESSION_ID")
	}
	store, err := app.NewStore(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	snap, err := engine.BuildSnapshot(ctx, cfg, store)
	if err != nil {
		return err
	}
	var selected *app.Task
	for i := range snap.Tasks {
		if snap.Tasks[i].ID == args[0] || snap.Tasks[i].Session.ID == args[0] || strings.HasPrefix(snap.Tasks[i].ID, args[0]) || strings.HasPrefix(snap.Tasks[i].Session.ID, args[0]) {
			selected = &snap.Tasks[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("task/session %q not found", args[0])
	}
	sum := summary.NewCodexSummarizer(cfg, store)
	if err := sum.Refresh(ctx, selected); err != nil {
		return err
	}
	fmt.Println(selected.Summary)
	return nil
}

func runOpen(cfg config.Config, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: cockpit open PANE_ID")
	}
	return wezterm.ActivatePane(context.Background(), cfg.WeztermBin, args[0])
}

func runAttach(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("attach", flag.ContinueOnError)
	paneFlag := fs.Int("pane", 0, "WezTerm pane id; defaults to WEZTERM_PANE")
	detach := fs.Bool("detach", false, "clear manual binding instead")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	paneID := *paneFlag
	if paneID == 0 {
		var err error
		paneID, err = currentPaneID()
		if err != nil && !*detach {
			return err
		}
	}
	store, err := app.NewStore(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := engine.BuildSnapshot(ctx, cfg, store)
	if err != nil {
		return err
	}
	task, err := selectAttachTask(snap.Tasks, snap.Panes, paneID, fs.Args())
	if err != nil {
		return err
	}
	if *detach {
		if err := store.ClearManualBinding(task.ID); err != nil {
			return err
		}
		fmt.Printf("detached %s\n", task.ID)
		return nil
	}
	if err := store.SetManualBinding(task.ID, paneID); err != nil {
		return err
	}
	fmt.Printf("attached %s to pane %d\n", task.ID, paneID)
	return nil
}

func currentPaneID() (int, error) {
	raw := os.Getenv("WEZTERM_PANE")
	if raw == "" {
		return 0, errors.New("WEZTERM_PANE is not set; pass --pane")
	}
	paneID, err := strconv.Atoi(raw)
	if err != nil || paneID <= 0 {
		return 0, fmt.Errorf("invalid WEZTERM_PANE %q", raw)
	}
	return paneID, nil
}

func selectAttachTask(tasks []app.Task, panes []app.Pane, paneID int, args []string) (app.Task, error) {
	if len(args) > 1 {
		return app.Task{}, errors.New("usage: cockpit attach [--pane PANE_ID] [TASK_OR_SESSION_ID]")
	}
	if len(args) == 1 {
		return findTask(tasks, args[0])
	}
	pane, ok := findPane(panes, paneID)
	if !ok {
		return app.Task{}, fmt.Errorf("pane %d not found", paneID)
	}
	var candidates []app.Task
	for _, task := range tasks {
		if task.Session.Internal || task.Archived || task.Ignored {
			continue
		}
		if task.Binding.Pane != nil && task.Binding.Pane.PaneID == paneID {
			return task, nil
		}
		if task.Binding.Pane == nil && relatedPath(task.Session.CWD, pane.CWD) {
			candidates = append(candidates, task)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) == 0 {
		return app.Task{}, fmt.Errorf("could not infer task for pane %d; pass a task/session id", paneID)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Session.LastEventAt.After(candidates[j].Session.LastEventAt)
	})
	var ids []string
	for _, task := range candidates {
		ids = append(ids, task.IDShort()+"("+string(task.Session.Agent)+":"+task.RepoName()+")")
	}
	return app.Task{}, fmt.Errorf("multiple possible tasks for pane %d: %s; pass one id", paneID, strings.Join(ids, ", "))
}

func findTask(tasks []app.Task, id string) (app.Task, error) {
	var matches []app.Task
	for _, task := range tasks {
		if task.ID == id || task.Session.ID == id || strings.HasPrefix(task.ID, id) || strings.HasPrefix(task.Session.ID, id) {
			matches = append(matches, task)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return app.Task{}, fmt.Errorf("task/session %q not found", id)
	}
	return app.Task{}, fmt.Errorf("task/session %q is ambiguous", id)
}

func findPane(panes []app.Pane, paneID int) (app.Pane, bool) {
	for _, pane := range panes {
		if pane.PaneID == paneID {
			return pane, true
		}
	}
	return app.Pane{}, false
}

func relatedPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	return a == b || strings.HasPrefix(a, b+string(filepath.Separator)) || strings.HasPrefix(b, a+string(filepath.Separator))
}

func runDoctor(cfg config.Config) error {
	checks := []struct {
		name string
		fn   func() error
	}{
		{"codex home", func() error { return dirExists(cfg.CodexHome) }},
		{"claude home", func() error { return dirExists(cfg.ClaudeHome) }},
		{"codex sessions", func() error { return hasGlob(filepath.Join(cfg.CodexHome, "sessions", "*", "*", "*", "*.jsonl")) }},
		{"claude projects", func() error { return dirExists(filepath.Join(cfg.ClaudeHome, "projects")) }},
		{"wezterm cli", func() error {
			_, err := exec.LookPath(cfg.WeztermBin)
			return err
		}},
		{"wezterm panes", func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := wezterm.ListPanes(ctx, cfg.WeztermBin)
			return err
		}},
		{"codex exec", func() error {
			cmd := exec.Command(cfg.CodexBin, "exec", "--help")
			cmd.Env = append(os.Environ(), "COCKPIT_INTERNAL=1")
			return cmd.Run()
		}},
	}
	for _, check := range checks {
		err := check.fn()
		if err != nil {
			fmt.Printf("FAIL %-16s %v\n", check.name, err)
		} else {
			fmt.Printf("OK   %-16s\n", check.name)
		}
	}
	return nil
}

func dirExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

func hasGlob(pattern string) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no matches for %s", pattern)
	}
	return nil
}
