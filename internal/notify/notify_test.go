package notify

import (
	"strings"
	"testing"
	"time"

	"cockpit/internal/app"
)

func TestNotificationStateCompleted(t *testing.T) {
	task := notificationTask(app.StatusCompleted)

	state := NotificationState(task, 20*time.Minute)

	if state != "completed" {
		t.Fatalf("state = %q, want completed", state)
	}
	message, ok := BuildStateMessage(task, state)
	if !ok {
		t.Fatal("expected message")
	}
	if message.Kind != "completed" || !strings.Contains(message.Title, "completed") {
		t.Fatalf("message = %#v, want completed kind/title", message)
	}
}

func TestNotificationStateStuckKeysByTraceID(t *testing.T) {
	task := notificationTask(app.StatusWorking)
	task.StatusExplain.ActiveTrace = &app.TraceRef{
		ID:        "call:slow",
		Kind:      app.TraceTool,
		Name:      "Bash",
		StartedAt: time.Now().Add(-30 * time.Minute),
	}

	state := NotificationState(task, 20*time.Minute)

	if state != "stuck:call:slow" {
		t.Fatalf("state = %q, want stuck trace state", state)
	}
	message, ok := BuildStateMessage(task, state)
	if !ok {
		t.Fatal("expected message")
	}
	if message.Kind != "stuck" || !strings.Contains(message.Body, "Bash") {
		t.Fatalf("message = %#v, want stuck kind/body", message)
	}
}

func TestNotificationMessageIncludesPaneHint(t *testing.T) {
	task := notificationTask(app.StatusCompleted)

	message, ok := BuildStateMessage(task, "completed")
	if !ok {
		t.Fatal("expected message")
	}
	if !strings.Contains(message.Body, "(pane:2)") {
		t.Fatalf("body = %q, want pane hint", message.Body)
	}
}

func TestNotificationMessageIncludesUnboundHint(t *testing.T) {
	task := notificationTask(app.StatusCompleted)
	task.Binding.Pane = nil

	message, ok := BuildStateMessage(task, "completed")
	if !ok {
		t.Fatal("expected message")
	}
	if !strings.Contains(message.Body, "(unbound)") || message.PaneID != nil {
		t.Fatalf("message = %#v, want unbound body and no pane id", message)
	}
}

func notificationTask(status app.Status) app.Task {
	return app.Task{
		ID: "codex:test",
		Session: app.Session{
			ID:    "test",
			Agent: app.AgentCodex,
			CWD:   "/tmp/example",
		},
		Binding: app.Binding{
			Pane: &app.Pane{PaneID: 2},
		},
		Status:  status,
		Summary: "summary",
	}
}
