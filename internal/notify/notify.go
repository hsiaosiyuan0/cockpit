package notify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"cockpit/internal/app"
)

var inputSoundCandidates = []string{
	"/System/Library/Sounds/Glass.aiff",
	"/System/Library/Sounds/Ping.aiff",
	"/System/Library/Sounds/Tink.aiff",
}

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
	kind := notificationKind(state)
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

func ShouldPlayInputSound(task app.Task, state string) bool {
	switch notificationKind(state) {
	case "waiting":
		return true
	case "attention", "blocked":
		return looksLikeInputRequired(task)
	default:
		return false
	}
}

func PlayInputSound(ctx context.Context) error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	player, err := exec.LookPath("afplay")
	if err != nil {
		return nil
	}
	sound := firstExistingInputSound()
	if sound == "" {
		return nil
	}
	return exec.CommandContext(ctx, player, sound).Run()
}

func notificationKind(state string) string {
	if strings.HasPrefix(state, "stuck:") {
		return "stuck"
	}
	return state
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

func looksLikeInputRequired(task app.Task) bool {
	parts := []string{
		task.AttentionReason,
		task.StatusExplain.Reason,
		task.StatusExplain.Rule,
		task.StatusExplain.RuleSeverity,
	}
	if task.StatusExplain.AttentionEvent != nil {
		parts = append(parts, task.StatusExplain.AttentionEvent.Text, task.StatusExplain.AttentionEvent.Detail)
	}
	for _, evidence := range task.StatusExplain.Evidence {
		parts = append(parts, evidence)
	}
	text := strings.ToLower(strings.Join(parts, " "))
	for _, pattern := range []string{
		"user input",
		"your reply",
		"asking for a decision",
		"decision",
		"confirm",
		"confirmation",
		"permission",
		"approval",
		"auth",
		"login",
		"credential",
		"need your",
		"requires your",
		"需要你",
		"请确认",
		"确认",
		"决定",
		"选择",
		"输入",
		"授权",
		"登录",
		"凭证",
		"要不要",
		"是否",
	} {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func firstExistingInputSound() string {
	for _, candidate := range inputSoundCandidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
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
