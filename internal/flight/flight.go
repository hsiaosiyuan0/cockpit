package flight

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cockpit/internal/app"
)

func Build(task app.Task) []app.FlightEvent {
	var events []app.FlightEvent
	for index, event := range task.Session.Events {
		title, detail := eventTitle(event)
		events = append(events, app.FlightEvent{
			ID:     fmt.Sprintf("event:%d", index),
			At:     event.At,
			Kind:   string(event.Type),
			Title:  title,
			Detail: detail,
			Source: "log",
		})
	}
	flattenTrace(&events, task.Session.Trace)
	if task.Status != "" {
		events = append(events, app.FlightEvent{
			ID:     "status:" + string(task.Status),
			At:     task.Session.LastEventAt,
			Kind:   "status",
			Title:  string(task.Status),
			Detail: task.StatusExplain.Reason,
			Status: string(task.Status),
			Source: "status",
		})
	}
	if task.Diff.Dirty {
		events = append(events, app.FlightEvent{
			ID:     "diff:" + task.Diff.RepoRoot,
			At:     task.Diff.GeneratedAt,
			Kind:   "diff",
			Title:  "diff radar",
			Detail: task.Diff.Summary,
			Source: "git",
		})
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].At.Before(events[j].At)
	})
	if len(events) > 80 {
		events = events[len(events)-80:]
	}
	return events
}

func flattenTrace(events *[]app.FlightEvent, spans []app.TraceSpan) {
	for _, span := range spans {
		at := span.StartedAt
		if at.IsZero() {
			at = span.EndedAt
		}
		title := span.Name
		if span.Kind == app.TraceAssistant {
			title = "assistant response"
		}
		detail := span.Detail
		if detail == "" {
			detail = span.Summary
		}
		*events = append(*events, app.FlightEvent{
			ID:             "trace:" + span.ID,
			ParentID:       span.ParentID,
			At:             at,
			Kind:           string(span.Kind),
			Title:          title,
			Detail:         detail,
			Status:         string(span.Status),
			DurationMillis: span.DurationMillis,
			Source:         "trace",
		})
		flattenTrace(events, span.Children)
	}
}

func eventTitle(event app.Event) (string, string) {
	text := clean(event.Text)
	detail := clean(event.Detail)
	switch event.Type {
	case app.EventUser:
		return "user prompt", text
	case app.EventAssistant:
		return "assistant reply", text
	case app.EventTool:
		return toolTitle(text), detail
	case app.EventResult:
		return "tool result", firstNonEmpty(detail, text)
	case app.EventError:
		return "error", firstNonEmpty(detail, text)
	case app.EventSystem:
		return "system", firstNonEmpty(detail, text)
	default:
		return string(event.Type), firstNonEmpty(detail, text)
	}
}

func toolTitle(text string) string {
	text = strings.TrimPrefix(text, "tool: ")
	text = strings.TrimPrefix(text, "exec: ")
	if text == "" {
		return "tool"
	}
	return text
}

func clean(text string) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	runes := []rune(text)
	if len(runes) > 240 {
		return string(runes[:237]) + "..."
	}
	return text
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func Since(event app.FlightEvent, now time.Time) time.Duration {
	if event.At.IsZero() {
		return 0
	}
	return now.Sub(event.At)
}
