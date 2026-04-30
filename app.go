package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	cockpitapp "cockpit/internal/app"
	"cockpit/internal/config"
	"cockpit/internal/engine"
	"cockpit/internal/notify"
	"cockpit/internal/summary"
	"cockpit/internal/wezterm"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx     context.Context
	cfg     config.Config
	store   *cockpitapp.Store
	initErr error

	mu                     sync.Mutex
	seenStatuses           map[string]cockpitapp.Status
	seenNotificationStates map[string]string
	notifyReady            bool
}

func NewApp() *App {
	cfg, err := config.Load()
	if err != nil {
		return &App{
			initErr:                err,
			seenStatuses:           map[string]cockpitapp.Status{},
			seenNotificationStates: map[string]string{},
		}
	}
	store, err := cockpitapp.NewStore(cfg)
	return &App{
		cfg:                    cfg,
		store:                  store,
		initErr:                err,
		seenStatuses:           map[string]cockpitapp.Status{},
		seenNotificationStates: map[string]string{},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.setupNotifications(ctx)
}

func (a *App) GetSnapshot() (cockpitapp.Snapshot, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cfg := a.currentConfig()
	snap, err := engine.BuildSnapshot(ctx, cfg, a.store)
	if err != nil {
		return snap, err
	}
	a.notifyTransitions(ctx, snap.Tasks)
	return snap, nil
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
		return config.Settings{}, err
	}
	a.cfg = nextCfg
	a.mu.Unlock()
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

func (a *App) RefreshSummary(taskID string) (cockpitapp.Task, error) {
	if err := a.ready(); err != nil {
		return cockpitapp.Task{}, err
	}
	cfg := a.currentConfig()
	if !cfg.EnableLLMSummary {
		return cockpitapp.Task{}, errors.New("LLM summary is disabled in settings")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	snap, err := engine.BuildSnapshot(ctx, cfg, a.store)
	if err != nil {
		return cockpitapp.Task{}, err
	}
	task, err := findTask(snap.Tasks, taskID)
	if err != nil {
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
		return task, err
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	snap, err := engine.BuildSnapshot(ctx, cfg, a.store)
	if err != nil {
		return cockpitapp.Task{}, err
	}
	task, err := findTask(snap.Tasks, taskID)
	if err != nil {
		return cockpitapp.Task{}, err
	}
	if task.Session.Internal {
		return cockpitapp.Task{}, errors.New("internal cockpit task cannot be debriefed")
	}
	summarizer := summary.NewCodexSummarizer(cfg, a.store)
	if err := summarizer.Debrief(ctx, &task); err != nil {
		return task, err
	}
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

func (a *App) notifyTransitions(ctx context.Context, tasks []cockpitapp.Task) {
	type transition struct {
		task  cockpitapp.Task
		state string
	}
	var transitions []transition
	a.mu.Lock()
	for _, task := range tasks {
		if task.Session.Internal || task.Archived || task.Ignored {
			continue
		}
		state := notify.NotificationState(task, a.cfg.StuckAfter)
		previous, seen := a.seenNotificationStates[task.ID]
		if seen && state != "" && state != previous && notificationEnabled(a.cfg, state) {
			transitions = append(transitions, transition{task: task, state: state})
		}
		a.seenStatuses[task.ID] = task.Status
		a.seenNotificationStates[task.ID] = state
	}
	a.mu.Unlock()
	for _, transition := range transitions {
		a.notifyTaskTransition(ctx, transition.task, transition.state)
	}
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

func (a *App) notifyTaskTransition(ctx context.Context, task cockpitapp.Task, state string) {
	message, ok := notify.BuildStateMessage(task, state)
	if !ok {
		return
	}
	if a.sendNativeNotification(message) {
		return
	}
	_ = notify.SendAppleScript(ctx, message)
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
