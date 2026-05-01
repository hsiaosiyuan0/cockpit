package main

import (
	"testing"
	"time"

	"cockpit/internal/app"
	"cockpit/internal/config"
)

func TestNotificationPaneID(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want int
		ok   bool
	}{
		{name: "float from json userInfo", data: map[string]interface{}{"pane_id": float64(7)}, want: 7, ok: true},
		{name: "int", data: map[string]interface{}{"pane_id": 2}, want: 2, ok: true},
		{name: "string", data: map[string]interface{}{"pane_id": "4"}, want: 4, ok: true},
		{name: "missing", data: map[string]interface{}{}, ok: false},
		{name: "negative", data: map[string]interface{}{"pane_id": float64(-1)}, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := notificationPaneID(test.data)
			if ok != test.ok {
				t.Fatalf("ok = %v, want %v", ok, test.ok)
			}
			if ok && got != test.want {
				t.Fatalf("paneID = %d, want %d", got, test.want)
			}
		})
	}
}

func TestNotificationTaskID(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want string
		ok   bool
	}{
		{name: "task id", data: map[string]interface{}{"task_id": "codex:test"}, want: "codex:test", ok: true},
		{name: "missing", data: map[string]interface{}{}, ok: false},
		{name: "empty", data: map[string]interface{}{"task_id": ""}, ok: false},
		{name: "wrong type", data: map[string]interface{}{"task_id": 12}, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := notificationTaskID(test.data)
			if ok != test.ok {
				t.Fatalf("ok = %v, want %v", ok, test.ok)
			}
			if ok && got != test.want {
				t.Fatalf("taskID = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInputSoundEnabledIndependentFromAttentionNotificationToggle(t *testing.T) {
	task := app.Task{
		Status:          app.StatusWaiting,
		StatusExplain:   app.StatusExplanation{Reason: "agent appears to be waiting for user input"},
		AttentionReason: "agent appears to be waiting for user input",
	}
	cfg := config.Config{
		NotifyAttention:  false,
		NotifyInputSound: true,
		NotificationMode: "normal",
	}

	if !inputSoundEnabled(cfg, task, "waiting") {
		t.Fatal("expected input sound to be independent from text notification toggle")
	}
}

func TestInputSoundDisabledInSilentMode(t *testing.T) {
	task := app.Task{
		Status:        app.StatusWaiting,
		StatusExplain: app.StatusExplanation{Reason: "agent appears to be waiting for user input"},
	}
	cfg := config.Config{
		NotifyInputSound: true,
		NotificationMode: "silent",
	}

	if inputSoundEnabled(cfg, task, "waiting") {
		t.Fatal("expected silent mode to disable input sound")
	}
}

func TestApplyInputAlertsAnnotatesRecentTask(t *testing.T) {
	createdAt := time.Now()
	task := app.Task{
		ID:     "codex:test",
		Status: app.StatusWaiting,
	}
	cockpit := &App{
		inputAlerts: map[string]inputAlert{
			task.ID: {At: createdAt, Reason: "agent appears to be waiting for user input"},
		},
	}
	tasks := []app.Task{task}

	cockpit.applyInputAlerts(tasks)

	if tasks[0].InputAlertAt == nil || tasks[0].InputAlertAt.IsZero() {
		t.Fatal("expected input alert timestamp")
	}
	if tasks[0].InputAlertReason == "" {
		t.Fatal("expected input alert reason")
	}
}

func TestApplyInputAlertsPrunesResolvedTask(t *testing.T) {
	task := app.Task{
		ID:     "codex:test",
		Status: app.StatusIdle,
	}
	cockpit := &App{
		inputAlerts: map[string]inputAlert{
			task.ID: {At: time.Now(), Reason: "agent appears to be waiting for user input"},
		},
	}
	tasks := []app.Task{task}

	cockpit.applyInputAlerts(tasks)

	if tasks[0].InputAlertAt != nil {
		t.Fatal("expected resolved task to remain unannotated")
	}
	if len(cockpit.inputAlerts) != 0 {
		t.Fatal("expected resolved task alert to be pruned")
	}
}
