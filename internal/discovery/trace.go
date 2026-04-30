package discovery

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"cockpit/internal/app"
)

var processExitPattern = regexp.MustCompile(`Process exited with code (-?\d+)`)

type traceBuilder struct {
	spans       map[string]*app.TraceSpan
	order       []string
	currentTurn string
	turnCount   int
}

func newTraceBuilder() *traceBuilder {
	return &traceBuilder{spans: map[string]*app.TraceSpan{}}
}

func (b *traceBuilder) startTurn(at time.Time, summary string) string {
	b.turnCount++
	id := fmt.Sprintf("turn:%d", b.turnCount)
	span := b.ensureSpan(id, app.TraceTurn, "user turn", "", at)
	span.Summary = compact(summary, 220)
	span.Status = app.TraceRunning
	b.currentTurn = id
	return id
}

func (b *traceBuilder) finishCurrentTurn(at time.Time) {
	if b.currentTurn == "" {
		return
	}
	b.finishSpan(b.currentTurn, at, nil, "", "")
	b.currentTurn = ""
}

func (b *traceBuilder) addAssistant(at time.Time, summary string) {
	if summary == "" {
		return
	}
	id := fmt.Sprintf("assistant:%d", len(b.order)+1)
	span := b.ensureSpan(id, app.TraceAssistant, "assistant", b.currentTurn, at)
	span.Summary = compact(summary, 220)
	b.finishSpan(id, at, nil, "", "")
}

func (b *traceBuilder) startTool(id string, name string, kind app.TraceKind, at time.Time, detail string) {
	if id == "" {
		id = fmt.Sprintf("tool:%d", len(b.order)+1)
	}
	if name == "" {
		name = "tool"
	}
	span := b.ensureSpan(id, kind, name, b.currentTurn, at)
	span.Detail = compact(detail, 260)
	span.Status = app.TraceRunning
}

func (b *traceBuilder) finishTool(id string, at time.Time, exitCode *int, summary string) {
	if id == "" {
		return
	}
	span := b.ensureSpan(id, app.TraceTool, "tool", b.currentTurn, at)
	if span.Name == "exec_command" && exitCode == nil && outputIndicatesRunning(summary) {
		if summary != "" {
			span.Summary = compact(summary, 260)
		}
		return
	}
	if span.Kind == app.TraceSubagent && span.Name == "spawn_agent" {
		if summary != "" {
			span.Summary = compact(summary, 260)
		}
		return
	}
	b.finishSpan(id, at, exitCode, summary, "")
}

func (b *traceBuilder) finishRunningSubagents(at time.Time, summary string) {
	for _, id := range b.order {
		span := b.spans[id]
		if span.Kind == app.TraceSubagent && span.Status == app.TraceRunning {
			b.finishSpan(id, at, nil, summary, "")
		}
	}
}

func (b *traceBuilder) ensureSpan(id string, kind app.TraceKind, name string, parentID string, at time.Time) *app.TraceSpan {
	if existing := b.spans[id]; existing != nil {
		if existing.Kind == "" {
			existing.Kind = kind
		}
		if existing.Name == "" || existing.Name == "tool" {
			existing.Name = name
		}
		if existing.ParentID == "" {
			existing.ParentID = parentID
		}
		if existing.StartedAt.IsZero() || (!at.IsZero() && at.Before(existing.StartedAt)) {
			existing.StartedAt = at
		}
		return existing
	}
	span := &app.TraceSpan{
		ID:        id,
		ParentID:  parentID,
		Kind:      kind,
		Name:      name,
		Status:    app.TraceRunning,
		StartedAt: at,
	}
	b.spans[id] = span
	b.order = append(b.order, id)
	return span
}

func (b *traceBuilder) finishSpan(id string, at time.Time, exitCode *int, summary string, detail string) {
	span := b.spans[id]
	if span == nil {
		return
	}
	span.EndedAt = at
	if exitCode != nil {
		code := *exitCode
		span.ExitCode = &code
		if code == 0 {
			span.Status = app.TraceSucceeded
		} else {
			span.Status = app.TraceFailed
		}
	} else if span.Status == app.TraceRunning {
		span.Status = app.TraceSucceeded
	}
	if !span.StartedAt.IsZero() && !span.EndedAt.IsZero() && !span.EndedAt.Before(span.StartedAt) {
		span.DurationMillis = span.EndedAt.Sub(span.StartedAt).Milliseconds()
	}
	if summary != "" {
		span.Summary = compact(summary, 260)
	}
	if detail != "" {
		span.Detail = compact(detail, 260)
	}
}

func (b *traceBuilder) finalize() []app.TraceSpan {
	children := map[string][]string{}
	var roots []string
	for _, id := range b.order {
		span := b.spans[id]
		if span == nil {
			continue
		}
		if span.ParentID != "" && b.spans[span.ParentID] != nil {
			children[span.ParentID] = append(children[span.ParentID], id)
		} else {
			roots = append(roots, id)
		}
	}
	out := make([]app.TraceSpan, 0, len(roots))
	for _, id := range roots {
		out = append(out, b.buildSpan(id, children))
	}
	return trimTraceRoots(out, 12, 140)
}

func (b *traceBuilder) buildSpan(id string, children map[string][]string) app.TraceSpan {
	source := b.spans[id]
	span := *source
	span.Children = nil
	for _, childID := range children[id] {
		span.Children = append(span.Children, b.buildSpan(childID, children))
	}
	if span.Kind == app.TraceTurn && span.Status == app.TraceRunning && !hasRunningWork(span.Children) {
		span.Status = app.TraceSucceeded
	}
	return span
}

func hasRunningWork(spans []app.TraceSpan) bool {
	for _, span := range spans {
		if span.Status == app.TraceRunning && (span.Kind == app.TraceTool || span.Kind == app.TraceSubagent) {
			return true
		}
		if hasRunningWork(span.Children) {
			return true
		}
	}
	return false
}

func trimTraceRoots(roots []app.TraceSpan, maxRoots int, maxSpans int) []app.TraceSpan {
	if len(roots) <= maxRoots && countTraceSpans(roots) <= maxSpans {
		return roots
	}
	keep := make([]app.TraceSpan, 0, len(roots))
	for _, root := range roots {
		if hasRunningWork([]app.TraceSpan{root}) {
			keep = append(keep, root)
		}
	}
	for i := len(roots) - 1; i >= 0 && len(keep) < maxRoots; i-- {
		if containsTraceRoot(keep, roots[i].ID) {
			continue
		}
		keep = append([]app.TraceSpan{roots[i]}, keep...)
	}
	for countTraceSpans(keep) > maxSpans && len(keep) > 1 {
		keep = keep[1:]
	}
	return keep
}

func countTraceSpans(spans []app.TraceSpan) int {
	count := 0
	for _, span := range spans {
		count++
		count += countTraceSpans(span.Children)
	}
	return count
}

func containsTraceRoot(roots []app.TraceSpan, id string) bool {
	for _, root := range roots {
		if root.ID == id {
			return true
		}
	}
	return false
}

func traceKindForTool(name string) app.TraceKind {
	switch strings.ToLower(name) {
	case "spawn_agent", "wait_agent", "send_input", "close_agent", "resume_agent", "task":
		return app.TraceSubagent
	default:
		return app.TraceTool
	}
}

func traceDetailForTool(name string, payload map[string]any) string {
	args := stringField(payload, "arguments")
	if args == "" {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return args
	}
	switch name {
	case "exec_command":
		return stringField(parsed, "cmd")
	case "write_stdin":
		return "session " + strings.TrimSpace(fmt.Sprint(parsed["session_id"]))
	case "spawn_agent":
		if msg := stringField(parsed, "message"); msg != "" {
			return msg
		}
	case "wait_agent":
		if targets := sliceField(parsed, "targets"); len(targets) > 0 {
			var ids []string
			for _, target := range targets {
				ids = append(ids, fmt.Sprint(target))
			}
			return strings.Join(ids, ", ")
		}
	}
	return args
}

func commandSummary(payload map[string]any) string {
	value := payload["command"]
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var parts []string
		for _, part := range typed {
			parts = append(parts, fmt.Sprint(part))
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func outputExitCode(output string) *int {
	match := processExitPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return nil
	}
	var code int
	if _, err := fmt.Sscanf(match[1], "%d", &code); err != nil {
		return nil
	}
	return &code
}

func outputIndicatesRunning(output string) bool {
	return strings.Contains(output, "Process running with session ID") || strings.Contains(output, "Process still running")
}

func intField(obj map[string]any, key string) *int {
	value, ok := obj[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case float64:
		v := int(typed)
		return &v
	case int:
		v := typed
		return &v
	default:
		return nil
	}
}
