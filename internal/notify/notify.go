package notify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"cockpit/internal/app"
)

type Message struct {
	ID     string
	Title  string
	Body   string
	TaskID string
	PaneID *int
	Status app.Status
	Kind   string
}

func MaybeNotify(ctx context.Context, task app.Task, previous app.Status) {
	message, ok := BuildMessage(task, previous)
	if !ok {
		return
	}
	_ = SendAppleScript(ctx, message)
}

func BuildMessage(task app.Task, previous app.Status) (Message, bool) {
	if previous == "" || previous == task.Status {
		return Message{}, false
	}
	state := NotificationState(task, 0)
	if state == "" {
		return Message{}, false
	}
	return BuildStateMessage(task, state)
}

func NotificationState(task app.Task, stuckAfter time.Duration) string {
	switch task.Status {
	case app.StatusNeedsAttention:
		return "attention"
	case app.StatusBlocked:
		return "blocked"
	case app.StatusDrift:
		return "stuck:" + task.ID
	case app.StatusWaiting:
		return "waiting"
	case app.StatusCompleted:
		return "completed"
	case app.StatusWorking:
		if stuckAfter > 0 && task.StatusExplain.ActiveTrace != nil && !task.StatusExplain.ActiveTrace.StartedAt.IsZero() && time.Since(task.StatusExplain.ActiveTrace.StartedAt) >= stuckAfter {
			return "stuck:" + task.StatusExplain.ActiveTrace.ID
		}
	}
	return ""
}

func BuildStateMessage(task app.Task, state string) (Message, bool) {
	if state == "" {
		return Message{}, false
	}
	kind := state
	if strings.HasPrefix(state, "stuck:") {
		kind = "stuck"
	}
	title := messageTitle(task, kind)
	body := messageBody(task, kind)
	message := Message{
		ID:     notificationID(task, kind),
		Title:  title,
		Body:   body,
		TaskID: task.ID,
		Status: task.Status,
		Kind:   kind,
	}
	if task.Binding.Pane != nil {
		paneID := task.Binding.Pane.PaneID
		message.PaneID = &paneID
	}
	return message, true
}

func messageTitle(task app.Task, kind string) string {
	switch kind {
	case "attention":
		return "Cockpit needs input: " + task.RepoName()
	case "waiting":
		return "Cockpit waiting: " + task.RepoName()
	case "completed":
		return "Cockpit completed: " + task.RepoName()
	case "blocked":
		return "Cockpit blocked: " + task.RepoName()
	case "stuck":
		return "Cockpit may be stuck: " + task.RepoName()
	default:
		return "Cockpit: " + string(task.Session.Agent) + " " + task.RepoName()
	}
}

func messageBody(task app.Task, kind string) string {
	var body string
	switch kind {
	case "attention":
		if task.AttentionReason != "" {
			body = task.AttentionReason
			break
		}
		if task.StatusExplain.Reason != "" {
			body = task.StatusExplain.Reason
			break
		}
	case "waiting":
		body = "agent appears to be waiting for your reply"
	case "completed":
		if task.Summary != "" {
			body = task.Summary
			break
		}
		body = "agent reported completion"
	case "blocked":
		if task.StatusExplain.Reason != "" {
			body = task.StatusExplain.Reason
			break
		}
		body = "agent appears blocked"
	case "stuck":
		if task.StatusExplain.ActiveTrace != nil {
			body = "long running " + task.StatusExplain.ActiveTrace.Name
			break
		}
		body = "agent has been working longer than the stuck threshold"
	}
	if body == "" {
		body = string(task.Status)
		if task.AttentionReason != "" {
			body += ": " + task.AttentionReason
		} else if task.Summary != "" {
			body += ": " + task.Summary
		}
	}
	return withBindingHint(body, task)
}

func withBindingHint(body string, task app.Task) string {
	body = truncateLine(body, 180)
	if task.Binding.Pane == nil {
		if body == "" {
			return "unbound"
		}
		return body + " (unbound)"
	}
	hint := fmt.Sprintf("pane:%d", task.Binding.Pane.PaneID)
	if body == "" {
		return hint
	}
	return body + " (" + hint + ")"
}

func truncateLine(text string, limit int) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	runes := []rune(text)
	if limit <= 3 || len(runes) <= limit {
		return text
	}
	return string(runes[:limit-3]) + "..."
}

func notificationID(task app.Task, kind string) string {
	id := strings.NewReplacer(":", "-", "/", "-", " ", "-").Replace(task.ID)
	return fmt.Sprintf("cockpit-%s-%s", id, kind)
}

func SendAppleScript(ctx context.Context, message Message) error {
	return osascript(ctx, message.Title, message.Body)
}

func osascript(ctx context.Context, title, body string) error {
	title = strings.ReplaceAll(title, `"`, `\"`)
	body = strings.ReplaceAll(body, `"`, `\"`)
	cmd := exec.CommandContext(ctx, "osascript", "-e", `display notification "`+body+`" with title "`+title+`"`)
	return cmd.Run()
}
