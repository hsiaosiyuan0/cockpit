package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	cockpitapp "cockpit/internal/app"
	"cockpit/internal/applog"
	"cockpit/internal/config"
	"cockpit/internal/discovery"
	"cockpit/internal/engine"
	"cockpit/internal/execpath"
	"cockpit/internal/macosactivity"
	"cockpit/internal/notify"
	"cockpit/internal/promptsearch"
	"cockpit/internal/summary"
	"cockpit/internal/version"
	"cockpit/internal/wezterm"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx     context.Context
	cfg     config.Config
	store   *cockpitapp.Store
	logger  *applog.Logger
	initErr error

	mu                     sync.Mutex
	snapshotMu             sync.Mutex
	share                  *readonlyServer
	seenStatuses           map[string]cockpitapp.Status
	seenNotificationStates map[string]string
	inputAlerts            map[string]inputAlert
	paneCache              []cockpitapp.Pane
	paneCacheAt            time.Time
	monitorCancel          context.CancelFunc
	lastSnapshotLogAt      time.Time
	lastSnapshotSignature  string
	lastPaneFallbackLogAt  time.Time
	notifyReady            bool
}

type inputAlert struct {
	At     time.Time
	Reason string
}

const (
	inputAlertTTL             = 15 * time.Minute
	backgroundMonitorInterval = 5 * time.Second
	paneCacheTTL              = 5 * time.Minute
)

func NewApp() *App {
	cfg, err := config.Load()
	if err != nil {
		return &App{
			initErr:                err,
			seenStatuses:           map[string]cockpitapp.Status{},
			seenNotificationStates: map[string]string{},
			inputAlerts:            map[string]inputAlert{},
		}
	}
	logger, logErr := applog.New(cfg.StateDir)
	if logErr == nil {
		logStartup(logger, cfg)
	} else {
		fmt.Printf("cockpit log unavailable: %v\n", logErr)
	}
	store, err := cockpitapp.NewStore(cfg)
	if err != nil && logger != nil {
		logger.Printf("store init failed: %v", err)
	}
	return &App{
		cfg:                    cfg,
		store:                  store,
		logger:                 logger,
		initErr:                err,
		seenStatuses:           map[string]cockpitapp.Status{},
		seenNotificationStates: map[string]string{},
		inputAlerts:            map[string]inputAlert{},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.logf("wails startup complete")
	if macosactivity.Begin("Cockpit monitors Claude and Codex sessions in the background") {
		a.logf("macos background activity started")
	}
	go a.setupNotifications(ctx)
	go a.warmPromptIndex()
	a.startBackgroundMonitor(ctx)
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	cancel := a.monitorCancel
	a.monitorCancel = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	macosactivity.End()
	a.logf("wails shutdown complete")
}

func (a *App) GetSnapshot() (cockpitapp.Snapshot, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return a.buildMonitoredSnapshot(ctx, "app", true)
}

func (a *App) startBackgroundMonitor(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	a.mu.Lock()
	if a.monitorCancel != nil {
		a.monitorCancel()
	}
	a.monitorCancel = cancel
	a.mu.Unlock()
	go a.backgroundMonitor(ctx)
}

func (a *App) backgroundMonitor(ctx context.Context) {
	a.logf("background monitor started interval=%s", backgroundMonitorInterval)
	timer := time.NewTimer(1200 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			a.logf("background monitor stopped")
			return
		case <-timer.C:
			runCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			if _, err := a.buildMonitoredSnapshot(runCtx, "background", true); err != nil {
				a.logf("background monitor refresh failed err=%v", err)
			}
			cancel()
			timer.Reset(backgroundMonitorInterval)
		}
	}
}

func (a *App) buildMonitoredSnapshot(ctx context.Context, source string, notify bool) (cockpitapp.Snapshot, error) {
	a.snapshotMu.Lock()
	defer a.snapshotMu.Unlock()
	cfg := a.currentConfig()
	fallbackPanes := a.cachedPanes()
	fallbackPanesUsed := false
	snap, err := engine.BuildSnapshotWithOptions(ctx, cfg, a.store, engine.SnapshotOptions{
		FallbackPanes:     fallbackPanes,
		FallbackPanesUsed: &fallbackPanesUsed,
	})
	if err != nil {
		a.logSnapshotOutcome(source, snap, err)
		return snap, err
	}
	if !fallbackPanesUsed {
		a.rememberPanes(snap.Panes)
	} else {
		a.logPaneFallback(source, len(snap.Panes))
	}
	a.logSnapshotOutcome(source, snap, nil)
	if notify {
		a.notifyTransitions(ctx, snap.Tasks)
	}
	a.applyInputAlerts(snap.Tasks)
	return snap, nil
}

func (a *App) cachedPanes() []cockpitapp.Pane {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.paneCache) == 0 || time.Since(a.paneCacheAt) > paneCacheTTL {
		return nil
	}
	return cloneAppPanes(a.paneCache)
}

func (a *App) rememberPanes(panes []cockpitapp.Pane) {
	if len(panes) == 0 {
		return
	}
	a.mu.Lock()
	a.paneCache = cloneAppPanes(panes)
	a.paneCacheAt = time.Now()
	a.mu.Unlock()
}

func cloneAppPanes(panes []cockpitapp.Pane) []cockpitapp.Pane {
	if len(panes) == 0 {
		return nil
	}
	cloned := make([]cockpitapp.Pane, len(panes))
	copy(cloned, panes)
	return cloned
}

func (a *App) logPaneFallback(source string, count int) {
	a.mu.Lock()
	shouldLog := time.Since(a.lastPaneFallbackLogAt) > time.Minute
	if shouldLog {
		a.lastPaneFallbackLogAt = time.Now()
	}
	a.mu.Unlock()
	if shouldLog {
		a.logf("snapshot used cached panes source=%s panes=%d", source, count)
	}
}

func (a *App) GetDemoSnapshot() cockpitapp.Snapshot {
	return cockpitapp.DemoSnapshot()
}

func (a *App) GetSettings() (config.Settings, error) {
	if err := a.ready(); err != nil {
		return config.Settings{}, err
	}
	return a.currentConfig().Settings(), nil
}

func (a *App) SaveSettings(settings config.Settings) (config.Settings, error) {
	if err := a.ready(); err != nil {
		return config.Settings{}, err
	}
	a.mu.Lock()
	nextCfg := config.ApplySettings(a.cfg, settings)
	saved := nextCfg.Settings()
	if err := config.SaveSettings(nextCfg.SettingsPath, saved); err != nil {
		a.mu.Unlock()
		a.logf("settings save failed err=%v", err)
		return config.Settings{}, err
	}
	a.cfg = nextCfg
	a.mu.Unlock()
	a.logf("settings saved codex_home=%s claude_home=%s codex_bin=%s resolved_codex=%s wezterm_bin=%s resolved_wezterm=%s", nextCfg.CodexHome, nextCfg.ClaudeHome, nextCfg.CodexBin, execpath.Resolve(nextCfg.CodexBin), nextCfg.WeztermBin, execpath.Resolve(nextCfg.WeztermBin))
	return saved, nil
}

func (a *App) OpenPane(paneID int) error {
	if err := a.ready(); err != nil {
		return err
	}
	if paneID < 0 {
		return fmt.Errorf("invalid pane id %d", paneID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.activatePane(ctx, paneID)
}

func (a *App) ArchiveTask(taskID string) (cockpitapp.Snapshot, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	if err := a.store.SetArchived(taskID, true); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	return a.GetSnapshot()
}

func (a *App) IgnoreTask(taskID string) (cockpitapp.Snapshot, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	if err := a.store.SetIgnored(taskID, true); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	return a.GetSnapshot()
}

func (a *App) RestoreTask(taskID string) (cockpitapp.Snapshot, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	if err := a.store.SetArchived(taskID, false); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	if err := a.store.SetIgnored(taskID, false); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	return a.GetSnapshot()
}

func (a *App) AttachTask(taskID string, paneID int) (cockpitapp.Snapshot, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	if paneID < 0 {
		return cockpitapp.Snapshot{}, fmt.Errorf("invalid pane id %d", paneID)
	}
	if err := a.store.SetManualBinding(taskID, paneID); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	return a.GetSnapshot()
}

func (a *App) DetachTask(taskID string) (cockpitapp.Snapshot, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	if err := a.store.ClearManualBinding(taskID); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	return a.GetSnapshot()
}

func (a *App) RenameMission(missionID string, name string) (cockpitapp.Snapshot, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	if strings.TrimSpace(missionID) == "" {
		return cockpitapp.Snapshot{}, errors.New("mission id is required")
	}
	if err := a.store.SetMissionName(missionID, name); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	a.logf("mission renamed mission=%s custom=%t", missionID, strings.TrimSpace(name) != "")
	return a.GetSnapshot()
}

func (a *App) ReviewDoneItem(itemID string) (cockpitapp.Snapshot, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	if err := a.store.SetDoneReviewed(itemID, true); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	return a.GetSnapshot()
}

func (a *App) ArchiveDoneItem(itemID string) (cockpitapp.Snapshot, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	if err := a.store.ArchiveDoneItem(itemID); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	return a.GetSnapshot()
}

func (a *App) SearchPrompts(query string) ([]cockpitapp.PromptSearchResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	results, err := a.searchPromptResults(ctx, query)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []cockpitapp.PromptSearchResult{}
	}
	a.logf("prompt search query_len=%d results=%d", len([]rune(strings.TrimSpace(query))), len(results))
	return results, nil
}

func (a *App) searchPromptResults(ctx context.Context, query string) ([]cockpitapp.PromptSearchResult, error) {
	index, synced, err := a.syncPromptIndex(ctx)
	if err != nil {
		return nil, err
	}
	defer index.Close()
	results, err := index.Search(query, 40)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []cockpitapp.PromptSearchResult{}
	}
	a.logf("prompt index search synced=%d results=%d", synced, len(results))
	return results, nil
}

func (a *App) ReportFrontendError(message string, stack string) {
	message = strings.TrimSpace(message)
	stack = strings.TrimSpace(stack)
	if message == "" {
		message = "unknown frontend error"
	}
	a.logf("frontend error message=%q stack=%q", compactLogString(message, 500), compactLogString(stack, 2500))
}

func (a *App) syncPromptIndex(ctx context.Context) (*promptsearch.Index, int, error) {
	cfg := a.currentConfig()
	if cfg.RecentWindow < promptsearch.Retention {
		cfg.RecentWindow = promptsearch.Retention
	}
	if cfg.MaxSessions < 1000 {
		cfg.MaxSessions = 1000
	}
	sessions, err := discovery.DiscoverPromptSessions(ctx, cfg)
	if err != nil {
		a.logf("prompt search discovery warning err=%v", err)
	}
	userState := a.store.LoadUserState()
	tasks := make([]cockpitapp.Task, 0, len(sessions))
	for _, session := range sessions {
		taskID := cockpitapp.TaskID(session)
		tasks = append(tasks, cockpitapp.Task{
			ID:       taskID,
			Session:  session,
			Archived: userState.Archived[taskID],
			Ignored:  userState.Ignored[taskID],
		})
	}
	snap := cockpitapp.Snapshot{BuiltAt: time.Now(), Tasks: tasks}
	index, err := promptsearch.Open(promptsearch.DBPath(cfg.StateDir))
	if err != nil {
		return nil, 0, err
	}
	synced, err := index.Sync(snap, promptsearch.Retention)
	if err != nil {
		_ = index.Close()
		return nil, 0, err
	}
	return index, synced, nil
}

func (a *App) warmPromptIndex() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	index, synced, err := a.syncPromptIndex(ctx)
	if index != nil {
		_ = index.Close()
	}
	if err != nil {
		a.logf("prompt index warmup failed err=%v", err)
		return
	}
	a.logf("prompt index warmup synced=%d retention=%s db=%s", synced, promptsearch.Retention, promptsearch.DBPath(a.currentConfig().StateDir))
}

func (a *App) RefreshSummary(taskID string) (cockpitapp.Task, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Task{}, err
	}
	cfg := a.currentConfig()
	if !cfg.EnableLLMSummary {
		return cockpitapp.Task{}, errors.New("LLM summary is disabled in settings")
	}
	a.logf("summary requested task=%s codex_bin=%s resolved_codex=%s", taskID, cfg.CodexBin, execpath.Resolve(cfg.CodexBin))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	snap, err := engine.BuildSnapshot(ctx, cfg, a.store)
	if err != nil {
		a.logf("summary snapshot failed task=%s err=%v snapshot_errors=%s", taskID, err, strings.Join(snap.Errors, " | "))
		return cockpitapp.Task{}, err
	}
	task, err := findTask(snap.Tasks, taskID)
	if err != nil {
		a.logf("summary task lookup failed task=%s err=%v", taskID, err)
		return cockpitapp.Task{}, err
	}
	if task.Session.Internal {
		return cockpitapp.Task{}, errors.New("internal cockpit task cannot be summarized")
	}
	if task.Archived || task.Ignored {
		return cockpitapp.Task{}, errors.New("hidden task cannot be summarized")
	}
	summarizer := summary.NewCodexSummarizer(cfg, a.store)
	if err := summarizer.Refresh(ctx, &task); err != nil {
		a.logf("summary failed task=%s err=%v", task.ID, err)
		return task, err
	}
	a.logf("summary completed task=%s", task.ID)
	return task, nil
}

func (a *App) GenerateDebrief(taskID string) (cockpitapp.Task, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Task{}, err
	}
	cfg := a.currentConfig()
	if !cfg.EnableLLMSummary {
		return cockpitapp.Task{}, errors.New("LLM summary is disabled in settings")
	}
	a.logf("debrief requested task=%s codex_bin=%s resolved_codex=%s", taskID, cfg.CodexBin, execpath.Resolve(cfg.CodexBin))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	snap, err := engine.BuildSnapshot(ctx, cfg, a.store)
	if err != nil {
		a.logf("debrief snapshot failed task=%s err=%v snapshot_errors=%s", taskID, err, strings.Join(snap.Errors, " | "))
		return cockpitapp.Task{}, err
	}
	task, err := findTask(snap.Tasks, taskID)
	if err != nil {
		a.logf("debrief task lookup failed task=%s err=%v", taskID, err)
		return cockpitapp.Task{}, err
	}
	if task.Session.Internal {
		return cockpitapp.Task{}, errors.New("internal cockpit task cannot be debriefed")
	}
	summarizer := summary.NewCodexSummarizer(cfg, a.store)
	if err := summarizer.Debrief(ctx, &task); err != nil {
		a.logf("debrief failed task=%s err=%v", task.ID, err)
		return task, err
	}
	a.logf("debrief completed task=%s", task.ID)
	return task, nil
}

func (a *App) ready() error {
	if a.initErr != nil {
		return a.initErr
	}
	if a.store == nil {
		return errors.New("cockpit store is not initialized")
	}
	return nil
}

func (a *App) currentConfig() config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

func (a *App) GetLogPath() string {
	if a.logger == nil {
		return ""
	}
	return a.logger.Path()
}

func (a *App) logf(format string, args ...any) {
	if a == nil || a.logger == nil {
		return
	}
	a.logger.Printf(format, args...)
}

func compactLogString(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "..."
}

func (a *App) logSnapshotOutcome(source string, snap cockpitapp.Snapshot, err error) {
	errorText := strings.Join(snap.Errors, " | ")
	signature := fmt.Sprintf("%s|tasks=%d|panes=%d|missions=%d|errors=%s|err=%v", source, len(snap.Tasks), len(snap.Panes), len(snap.Missions), errorText, err)
	a.mu.Lock()
	shouldLog := err != nil || errorText != "" || signature != a.lastSnapshotSignature || time.Since(a.lastSnapshotLogAt) > time.Minute
	if shouldLog {
		a.lastSnapshotSignature = signature
		a.lastSnapshotLogAt = time.Now()
	}
	a.mu.Unlock()
	if !shouldLog {
		return
	}
	if err != nil {
		a.logf("snapshot failed source=%s tasks=%d panes=%d errors=%s err=%v", source, len(snap.Tasks), len(snap.Panes), errorText, err)
		return
	}
	if errorText != "" {
		a.logf("snapshot completed with errors source=%s tasks=%d panes=%d errors=%s", source, len(snap.Tasks), len(snap.Panes), errorText)
		return
	}
	a.logf("snapshot completed source=%s tasks=%d panes=%d missions=%d", source, len(snap.Tasks), len(snap.Panes), len(snap.Missions))
}

func logStartup(logger *applog.Logger, cfg config.Config) {
	logger.Printf("cockpit starting version=%s commit=%s build_time=%s pid=%d go=%s/%s debug=%t", version.Version, version.Commit, version.BuildTime, os.Getpid(), goruntime.GOOS, goruntime.GOARCH, debugLoggingEnabled())
	logger.Printf("path %s", startupPathLog(os.Getenv("PATH")))
	logger.Printf("state_dir=%s settings_path=%s internal_run_dir=%s", cfg.StateDir, cfg.SettingsPath, cfg.InternalRunDir)
	logger.Printf("homes codex=%s claude=%s", cfg.CodexHome, cfg.ClaudeHome)
	logger.Printf("bins codex=%s resolved_codex=%s wezterm=%s resolved_wezterm=%s", cfg.CodexBin, execpath.Resolve(cfg.CodexBin), cfg.WeztermBin, execpath.Resolve(cfg.WeztermBin))
}

func debugLoggingEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("COCKPIT_DEBUG"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func startupPathLog(pathValue string) string {
	if debugLoggingEnabled() {
		return "full=" + pathValue
	}
	if pathValue == "" {
		return "entries=0"
	}
	entries := strings.Split(pathValue, ":")
	shown := entries
	if len(shown) > 6 {
		shown = shown[:6]
	}
	return fmt.Sprintf("entries=%d sample=%s", len(entries), strings.Join(shown, ":"))
}

func (a *App) notifyTransitions(ctx context.Context, tasks []cockpitapp.Task) {
	type transition struct {
		task             cockpitapp.Task
		state            string
		sendNotification bool
		playSound        bool
	}
	var transitions []transition
	a.mu.Lock()
	cfg := a.cfg
	now := time.Now()
	a.pruneInputAlertsLocked(now, tasks)
	for _, task := range tasks {
		if task.Session.Internal || task.Archived || task.Ignored {
			continue
		}
		state := notify.NotificationState(task, cfg.StuckAfter)
		previous, seen := a.seenNotificationStates[task.ID]
		sendNotification := notificationEnabled(cfg, state)
		playSound := inputSoundEnabled(cfg, task, state)
		if seen && state != "" && state != previous && (sendNotification || playSound) {
			transitions = append(transitions, transition{task: task, state: state, sendNotification: sendNotification, playSound: playSound})
			if playSound {
				if a.inputAlerts == nil {
					a.inputAlerts = map[string]inputAlert{}
				}
				a.inputAlerts[task.ID] = inputAlert{
					At:     now,
					Reason: inputAlertReason(task),
				}
			}
		}
		a.seenStatuses[task.ID] = task.Status
		a.seenNotificationStates[task.ID] = state
	}
	a.mu.Unlock()
	for _, transition := range transitions {
		a.notifyTaskTransition(ctx, transition.task, transition.state, transition.playSound, transition.sendNotification)
	}
}

func (a *App) applyInputAlerts(tasks []cockpitapp.Task) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.inputAlerts) == 0 {
		return
	}
	now := time.Now()
	a.pruneInputAlertsLocked(now, tasks)
	for index := range tasks {
		alert, ok := a.inputAlerts[tasks[index].ID]
		if !ok {
			continue
		}
		alertAt := alert.At
		tasks[index].InputAlertAt = &alertAt
		tasks[index].InputAlertReason = alert.Reason
	}
}

func (a *App) pruneInputAlertsLocked(now time.Time, tasks []cockpitapp.Task) {
	if len(a.inputAlerts) == 0 {
		return
	}
	active := map[string]bool{}
	for _, task := range tasks {
		if task.Session.Internal || task.Archived || task.Ignored || !inputAlertStatus(task.Status) {
			continue
		}
		active[task.ID] = true
	}
	for taskID, alert := range a.inputAlerts {
		if !active[taskID] || (!alert.At.IsZero() && now.Sub(alert.At) > inputAlertTTL) {
			delete(a.inputAlerts, taskID)
		}
	}
}

func inputAlertStatus(status cockpitapp.Status) bool {
	return status == cockpitapp.StatusWaiting || status == cockpitapp.StatusNeedsAttention || status == cockpitapp.StatusBlocked
}

func inputAlertReason(task cockpitapp.Task) string {
	if task.AttentionReason != "" {
		return task.AttentionReason
	}
	if task.StatusExplain.Reason != "" {
		return task.StatusExplain.Reason
	}
	return "waiting for user input"
}

func notificationEnabled(cfg config.Config, state string) bool {
	if cfg.NotificationMode == "silent" {
		return false
	}
	if quietHoursActive(cfg.QuietHours, time.Now()) && state == "completed" {
		return false
	}
	if cfg.NotificationMode == "focus" && state != "attention" && state != "waiting" && state != "blocked" {
		return false
	}
	switch {
	case state == "attention", state == "waiting", state == "blocked":
		return cfg.NotifyAttention
	case state == "completed":
		return cfg.NotifyCompleted
	case strings.HasPrefix(state, "stuck:"):
		return cfg.NotifyStuck
	default:
		return false
	}
}

func inputSoundEnabled(cfg config.Config, task cockpitapp.Task, state string) bool {
	if cfg.NotificationMode == "silent" {
		return false
	}
	return cfg.NotifyInputSound && notify.ShouldPlayInputSound(task, state)
}

func quietHoursActive(quiet config.QuietHours, now time.Time) bool {
	if !quiet.Enabled {
		return false
	}
	start, okStart := parseClockMinute(quiet.Start)
	end, okEnd := parseClockMinute(quiet.End)
	if !okStart || !okEnd {
		return false
	}
	current := now.Hour()*60 + now.Minute()
	if start == end {
		return true
	}
	if start < end {
		return current >= start && current < end
	}
	return current >= start || current < end
}

func parseClockMinute(raw string) (int, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

func (a *App) setupNotifications(ctx context.Context) {
	if !wailsruntime.IsNotificationAvailable(ctx) {
		return
	}
	if err := wailsruntime.InitializeNotifications(ctx); err != nil {
		return
	}
	wailsruntime.OnNotificationResponse(ctx, a.handleNotificationResponse)
	authorized, err := wailsruntime.CheckNotificationAuthorization(ctx)
	if err == nil && !authorized {
		authorized, _ = wailsruntime.RequestNotificationAuthorization(ctx)
	}
	if !authorized {
		return
	}
	_ = wailsruntime.RegisterNotificationCategory(ctx, wailsruntime.NotificationCategory{
		ID: "cockpit-task",
		Actions: []wailsruntime.NotificationAction{
			{ID: "open", Title: "Open Pane"},
		},
	})
	a.mu.Lock()
	a.notifyReady = true
	a.mu.Unlock()
}

func (a *App) notifyTaskTransition(ctx context.Context, task cockpitapp.Task, state string, playSound bool, sendNotification bool) {
	if playSound {
		a.playInputSound()
	}
	if !sendNotification {
		return
	}
	message, ok := notify.BuildStateMessage(task, state)
	if !ok {
		return
	}
	if a.sendNativeNotification(message) {
		return
	}
	_ = notify.SendAppleScript(ctx, message)
}

func (a *App) playInputSound() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if err := notify.PlayInputSound(ctx); err != nil {
			a.logf("input sound failed err=%v", err)
		}
	}()
}

func (a *App) sendNativeNotification(message notify.Message) bool {
	a.mu.Lock()
	ready := a.notifyReady
	a.mu.Unlock()
	if a.ctx == nil || !ready {
		return false
	}
	data := map[string]interface{}{
		"task_id": message.TaskID,
		"status":  string(message.Status),
		"kind":    message.Kind,
	}
	if message.PaneID != nil {
		data["pane_id"] = *message.PaneID
	}
	options := wailsruntime.NotificationOptions{
		ID:         message.ID,
		Title:      message.Title,
		Body:       message.Body,
		CategoryID: "cockpit-task",
		Data:       data,
	}
	if err := wailsruntime.SendNotificationWithActions(a.ctx, options); err != nil {
		return false
	}
	return true
}

func (a *App) handleNotificationResponse(result wailsruntime.NotificationResult) {
	if result.Error != nil {
		return
	}
	paneID, ok := notificationPaneID(result.Response.UserInfo)
	if ok {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.activatePane(ctx, paneID)
		return
	}
	taskID, ok := notificationTaskID(result.Response.UserInfo)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = a.openTaskPane(ctx, taskID)
}

func (a *App) openTaskPane(ctx context.Context, taskID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	cfg := a.currentConfig()
	snap, err := engine.BuildSnapshot(ctx, cfg, a.store)
	if err != nil {
		return err
	}
	task, err := findTask(snap.Tasks, taskID)
	if err != nil {
		return err
	}
	if task.Binding.Pane == nil {
		return errors.New("notification task is not bound to a pane")
	}
	return a.activatePane(ctx, task.Binding.Pane.PaneID)
}

func (a *App) activatePane(ctx context.Context, paneID int) error {
	cfg := a.currentConfig()
	return wezterm.ActivatePane(ctx, cfg.WeztermBin, strconv.Itoa(paneID))
}

func notificationPaneID(data map[string]interface{}) (int, bool) {
	value, ok := data["pane_id"]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return int(typed), typed >= 0
	case int:
		return typed, typed >= 0
	case string:
		paneID, err := strconv.Atoi(typed)
		return paneID, err == nil && paneID >= 0
	default:
		return 0, false
	}
}

func notificationTaskID(data map[string]interface{}) (string, bool) {
	value, ok := data["task_id"]
	if !ok {
		return "", false
	}
	taskID, ok := value.(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return "", false
	}
	return taskID, true
}

func findTask(tasks []cockpitapp.Task, id string) (cockpitapp.Task, error) {
	var matches []cockpitapp.Task
	for _, task := range tasks {
		if task.ID == id || task.Session.ID == id || strings.HasPrefix(task.ID, id) || strings.HasPrefix(task.Session.ID, id) {
			matches = append(matches, task)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return cockpitapp.Task{}, fmt.Errorf("task/session %q not found", id)
	}
	return cockpitapp.Task{}, fmt.Errorf("task/session %q is ambiguous", id)
}
