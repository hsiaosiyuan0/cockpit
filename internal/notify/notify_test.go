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

func TestShouldPlayInputSoundForWaiting(t *testing.T) {
	task := notificationTask(app.StatusWaiting)

	if !ShouldPlayInputSound(task, "waiting") {
		t.Fatal("expected waiting task to play input sound")
	}
}

func TestShouldPlayInputSoundForDecisionAttention(t *testing.T) {
	task := notificationTask(app.StatusNeedsAttention)
	task.AttentionReason = "agent is asking for a decision"
	task.StatusExplain = app.StatusExplanation{
		Reason: "agent is asking for a decision",
		Rule:   "attention_pattern",
	}

	if !ShouldPlayInputSound(task, "attention") {
		t.Fatal("expected decision attention to play input sound")
	}
}

func TestShouldPlayInputSoundSkipsNonInteractiveAttention(t *testing.T) {
	task := notificationTask(app.StatusNeedsAttention)
	task.AttentionReason = "tests failed"
	task.StatusExplain = app.StatusExplanation{
		Reason: "tests failed",
		Rule:   "attention_pattern",
	}

	if ShouldPlayInputSound(task, "attention") {
		t.Fatal("expected non-interactive attention to skip input sound")
	}
}

func TestShouldPlayInputSoundForPermissionBlocked(t *testing.T) {
	task := notificationTask(app.StatusBlocked)
	task.AttentionReason = "permission or approval needed"
	task.StatusExplain = app.StatusExplanation{
		Reason:       "permission or approval needed",
		RuleSeverity: "blocked",
	}

	if !ShouldPlayInputSound(task, "blocked") {
		t.Fatal("expected permission blocked state to play input sound")
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
