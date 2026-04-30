package state

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"cockpit/internal/app"
	"cockpit/internal/config"
)

type Options struct {
	IdleAfter      time.Duration
	StuckAfter     time.Duration
	AttentionRules []config.AttentionRule
}

func Evaluate(task app.Task, idleAfter time.Duration) (app.Status, string) {
	explanation := Explain(task, idleAfter)
	return explanation.Status, explanation.Reason
}

func Explain(task app.Task, idleAfter time.Duration) app.StatusExplanation {
	return ExplainWithOptions(task, Options{IdleAfter: idleAfter})
}

func ExplainWithOptions(task app.Task, options Options) app.StatusExplanation {
	if options.IdleAfter <= 0 {
		options.IdleAfter = 10 * time.Minute
	}
	explanation := app.StatusExplanation{
		LastEventAt: task.Session.LastEventAt,
		Evidence:    []string{},
	}
	latestAssistant := latestAssistantAt(task.Session.Events, task.Session.Trace)
	explanation.LatestAssistantAt = latestAssistant
	explanation.BackgroundRunTraces = backgroundRunningTraces(task.Session.Trace, latestAssistant)

	if task.Session.Internal {
		explanation.Status = app.StatusUnknown
		explanation.Rule = "internal"
		explanation.Reason = "internal cockpit session is ignored"
		explanation.WhyNotWorking = "internal cockpit sessions are filtered from status decisions"
		explanation.WhyNotIdle = "internal sessions do not participate in idle decisions"
		return explanation
	}
	if task.Binding.Pane == nil {
		explanation.Status = app.StatusUnbound
		explanation.Rule = "binding"
		explanation.Reason = "no reliable WezTerm pane binding"
		explanation.WhyNotWorking = "task is not treated as active until it has a reliable WezTerm pane binding"
		explanation.WhyNotIdle = "unbound sessions are link targets first, not runtime state"
		explanation.Evidence = append(explanation.Evidence, "no pane binding")
		return explanation
	}
	if trace := activeRunningTrace(task.Session.Trace, latestAssistant); trace != nil {
		if options.StuckAfter > 0 && !trace.StartedAt.IsZero() && time.Since(trace.StartedAt) >= options.StuckAfter {
			explanation.Status = app.StatusDrift
			explanation.Rule = "long_running_trace"
			explanation.RuleSeverity = "drift"
			explanation.Reason = fmt.Sprintf("%s has been running for %s", trace.Name, roundDuration(time.Since(trace.StartedAt)))
			explanation.ActiveTrace = trace
			explanation.WhyNotIdle = "a tool or subagent is still running"
			explanation.Evidence = append(explanation.Evidence, traceEvidence("long running trace", *trace))
			return explanation
		}
		explanation.Status = app.StatusWorking
		explanation.Rule = "trace_active"
		explanation.Reason = "running " + trace.Name
		explanation.ActiveTrace = trace
		explanation.WhyNotIdle = "a tool or subagent is still running"
		explanation.Evidence = append(explanation.Evidence, traceEvidence("active trace", *trace))
		return explanation
	}
	explanation.WhyNotWorking = whyNotWorking(task.Session.Trace, latestAssistant)
	if reason := repeatedFailureReason(task.Session.Events, task.Session.Trace); reason != "" {
		explanation.Status = app.StatusBlocked
		explanation.Rule = "repeated_failure"
		explanation.RuleSeverity = "blocked"
		explanation.Reason = reason
		explanation.Evidence = append(explanation.Evidence, reason)
		return explanation
	}
	if status, reason, event, rule, severity := attentionReason(task.Session.Events, task.Session.Trace, options.AttentionRules); reason != "" {
		explanation.Status = status
		explanation.Rule = rule
		explanation.RuleSeverity = severity
		explanation.Reason = reason
		explanation.AttentionEvent = event
		explanation.Evidence = append(explanation.Evidence, reason)
		if event != nil {
			explanation.Evidence = append(explanation.Evidence, eventEvidence("attention source", *event))
		}
		return explanation
	}
	if isWaiting(task.Session.Events) {
		explanation.Status = app.StatusWaiting
		explanation.Rule = "assistant_question"
		explanation.Reason = "agent appears to be waiting for user input"
		explanation.Evidence = append(explanation.Evidence, "latest assistant message looks like a question")
		return explanation
	}
	if isCompleted(task.Session.Events) {
		explanation.Status = app.StatusCompleted
		explanation.Rule = "completion_phrase"
		explanation.Reason = "latest assistant message looks completed"
		explanation.Evidence = append(explanation.Evidence, "completion phrase in latest assistant message")
		return explanation
	}
	if status, reason, ok := idleFromLatestEvent(task.Session.Events, options.IdleAfter); ok {
		explanation.Status = status
		explanation.Rule = "idle_timeout"
		explanation.Reason = reason
		if latestAssistant.IsZero() {
			explanation.Evidence = append(explanation.Evidence, "latest significant event exceeded idle threshold")
		} else {
			explanation.Evidence = append(explanation.Evidence, "assistant response is older than settle window")
		}
		for _, trace := range explanation.BackgroundRunTraces {
			explanation.Evidence = append(explanation.Evidence, traceEvidence("background running trace", trace))
		}
		return explanation
	}
	explanation.Status = app.StatusWorking
	explanation.Rule = "recent_activity"
	explanation.Reason = "recent agent activity"
	explanation.WhyNotIdle = "latest significant event is still inside the settle window"
	explanation.Evidence = append(explanation.Evidence, "latest significant event is still fresh")
	return explanation
}

func RuleSummary(task app.Task) string {
	events := task.Session.Events
	if len(events) == 0 {
		return "no recent events parsed yet"
	}
	var tools, results int
	var lastAssistant, lastTool, lastUser string
	for _, event := range events {
		switch event.Type {
		case app.EventTool:
			tools++
			lastTool = event.Text
			if event.Detail != "" {
				lastTool += " " + event.Detail
			}
		case app.EventResult:
			results++
		case app.EventAssistant:
			lastAssistant = event.Text
		case app.EventUser:
			lastUser = event.Text
		}
	}
	switch {
	case task.AttentionReason != "":
		return task.AttentionReason
	case lastAssistant != "":
		return truncateRunes(lastAssistant, 130)
	case lastTool != "":
		return fmt.Sprintf("recently ran %s", truncateRunes(lastTool, 90))
	case tools > 0 || results > 0:
		return fmt.Sprintf("parsed %d tool events and %d results", tools, results)
	case lastUser != "":
		return "last user prompt: " + truncateRunes(lastUser, 100)
	default:
		return truncateRunes(events[len(events)-1].Text, 130)
	}
}

func activeRunningTrace(spans []app.TraceSpan, latestAssistant time.Time) *app.TraceRef {
	for _, span := range spans {
		if span.Status == app.TraceRunning {
			switch span.Kind {
			case app.TraceSubagent:
				ref := traceRef(span)
				return &ref
			case app.TraceTool:
				if latestAssistant.IsZero() || span.StartedAt.IsZero() || !latestAssistant.After(span.StartedAt) {
					ref := traceRef(span)
					return &ref
				}
			}
		}
		if ref := activeRunningTrace(span.Children, latestAssistant); ref != nil {
			return ref
		}
	}
	return nil
}

func backgroundRunningTraces(spans []app.TraceSpan, latestAssistant time.Time) []app.TraceRef {
	if latestAssistant.IsZero() {
		return nil
	}
	var refs []app.TraceRef
	for _, span := range spans {
		if span.Status == app.TraceRunning && span.Kind == app.TraceTool && !span.StartedAt.IsZero() && latestAssistant.After(span.StartedAt) {
			refs = append(refs, traceRef(span))
		}
		refs = append(refs, backgroundRunningTraces(span.Children, latestAssistant)...)
	}
	return refs
}

func whyNotWorking(spans []app.TraceSpan, latestAssistant time.Time) string {
	background := backgroundRunningTraces(spans, latestAssistant)
	if len(background) > 0 {
		if len(background) == 1 {
			return "running " + background[0].Name + " started before the latest assistant response, so it is treated as background"
		}
		return fmt.Sprintf("%d running tools started before the latest assistant response, so they are treated as background", len(background))
	}
	return "no running tool or subagent trace is blocking the current turn"
}

func traceRef(span app.TraceSpan) app.TraceRef {
	return app.TraceRef{
		ID:        span.ID,
		Kind:      span.Kind,
		Name:      span.Name,
		StartedAt: span.StartedAt,
		Summary:   span.Summary,
		Detail:    span.Detail,
	}
}

func traceEvidence(prefix string, trace app.TraceRef) string {
	name := string(trace.Kind) + " " + trace.Name
	if trace.Detail != "" {
		name += " (" + truncateRunes(trace.Detail, 80) + ")"
	}
	return prefix + ": " + name
}

func eventEvidence(prefix string, event app.Event) string {
	text := event.Text
	if event.Detail != "" {
		text += " " + event.Detail
	}
	return prefix + ": " + string(event.Type) + " " + truncateRunes(text, 120)
}

func latestAssistantAt(events []app.Event, spans []app.TraceSpan) time.Time {
	var latest time.Time
	for _, event := range events {
		if event.Type == app.EventAssistant && event.At.After(latest) {
			latest = event.At
		}
	}
	return latestAssistantSpanAt(spans, latest)
}

func latestAssistantSpanAt(spans []app.TraceSpan, latest time.Time) time.Time {
	for _, span := range spans {
		if span.Kind == app.TraceAssistant {
			at := span.EndedAt
			if at.IsZero() {
				at = span.StartedAt
			}
			if at.After(latest) {
				latest = at
			}
		}
		latest = latestAssistantSpanAt(span.Children, latest)
	}
	return latest
}

func attentionReason(events []app.Event, spans []app.TraceSpan, rules []config.AttentionRule) (app.Status, string, *app.Event, string, string) {
	for i := len(events) - 1; i >= 0 && i >= len(events)-20; i-- {
		event := events[i]
		if shouldIgnoreAttentionEvent(events, i, spans) {
			continue
		}
		if status, reason, rule, severity := attentionPatternReason(event, rules); reason != "" {
			return status, reason, &event, rule, severity
		}
	}
	if event, ok := latestSignificant(events); ok && event.Type == app.EventAssistant {
		if isDecisionQuestionText(event.Text) {
			return app.StatusNeedsAttention, "agent is asking for a decision", &event, "attention_pattern", "attention"
		}
	}
	return "", "", nil, "", ""
}

func attentionPatternReason(event app.Event, rules []config.AttentionRule) (app.Status, string, string, string) {
	text := strings.ToLower(event.Text + " " + event.Detail)
	if isExpectedFailureText(text) {
		return "", "", "", ""
	}
	configuredRules := len(rules) > 0
	if event.Type == app.EventError && !configuredRules {
		return app.StatusNeedsAttention, "error detected", "attention_pattern", "attention"
	}
	for _, rule := range activeRules(rules) {
		if !rule.Enabled || strings.TrimSpace(rule.Pattern) == "" {
			continue
		}
		if event.Type == app.EventAssistant && rule.ID == "runtime_error" {
			continue
		}
		if ruleMatches(rule.Pattern, text) {
			return statusForSeverity(rule.Severity), ruleMessage(rule), "attention_rule:" + rule.ID, rule.Severity
		}
	}
	if configuredRules {
		return "", "", "", ""
	}
	if event.Type == app.EventAssistant {
		switch {
		case strings.Contains(text, "test failed"), strings.Contains(text, "tests failed"), strings.Contains(text, "failing test"):
			return app.StatusNeedsAttention, "tests failed", "attention_pattern", "attention"
		case strings.Contains(text, "merge conflict"), strings.Contains(text, "permission denied"), strings.Contains(text, "requires approval"):
			return app.StatusNeedsAttention, resultAttentionReason(text), "attention_pattern", "attention"
		case strings.Contains(text, "tool_use_error"), strings.Contains(text, "blocked:"):
			return app.StatusNeedsAttention, "tool blocked", "attention_pattern", "attention"
		}
	}
	if event.Type == app.EventResult {
		if reason := resultAttentionReason(text); reason != "" {
			return app.StatusNeedsAttention, reason, "attention_pattern", "attention"
		}
	}
	return "", "", "", ""
}

func resultAttentionReason(text string) string {
	switch {
	case strings.Contains(text, "test failed"), strings.Contains(text, "tests failed"), strings.Contains(text, "failing test"):
		return "tests failed"
	case strings.Contains(text, "build failed"), strings.Contains(text, "compilation failed"):
		return "build failed"
	case strings.Contains(text, "error:"), strings.Contains(text, "exception"), strings.Contains(text, "traceback"), strings.Contains(text, "panic:"):
		return "error detected"
	case strings.Contains(text, "merge conflict"), strings.Contains(text, "conflict"):
		return "merge conflict detected"
	case strings.Contains(text, "permission denied"), strings.Contains(text, "requires approval"):
		return "permission or approval needed"
	default:
		return ""
	}
}

func activeRules(rules []config.AttentionRule) []config.AttentionRule {
	return rules
}

func ruleMatches(pattern, text string) bool {
	if compiled, err := regexp.Compile("(?i)" + pattern); err == nil {
		return compiled.MatchString(text)
	}
	return strings.Contains(text, strings.ToLower(pattern))
}

func statusForSeverity(severity string) app.Status {
	switch severity {
	case "blocked":
		return app.StatusBlocked
	case "drift":
		return app.StatusDrift
	default:
		return app.StatusNeedsAttention
	}
}

func ruleMessage(rule config.AttentionRule) string {
	if strings.TrimSpace(rule.Message) != "" {
		return rule.Message
	}
	if strings.TrimSpace(rule.Name) != "" {
		return strings.ToLower(rule.Name)
	}
	return "attention rule matched"
}

func repeatedFailureReason(events []app.Event, spans []app.TraceSpan) string {
	counts := map[string]int{}
	for i := len(events) - 1; i >= 0 && i >= len(events)-18; i-- {
		event := events[i]
		if event.Type != app.EventResult && event.Type != app.EventError {
			continue
		}
		if shouldIgnoreAttentionEvent(events, i, spans) {
			continue
		}
		text := strings.ToLower(event.Text + " " + event.Detail)
		reason := resultAttentionReason(text)
		if reason == "" {
			continue
		}
		counts[reason]++
		if counts[reason] >= 2 {
			return "same failure repeated: " + reason
		}
	}
	return ""
}

func shouldIgnoreAttentionEvent(events []app.Event, index int, spans []app.TraceSpan) bool {
	event := events[index]
	if isBenignSystemEvent(event) {
		return true
	}
	if event.Type != app.EventResult {
		return false
	}
	text := strings.ToLower(event.Text + " " + event.Detail)
	if strings.Contains(text, "process exited with code 0") {
		return true
	}
	if resultFromReadOnlyTrace(event, spans) {
		return true
	}
	return resultFromReadOnlyPreviousTool(events, index)
}

func isBenignSystemEvent(event app.Event) bool {
	if event.Type != app.EventSystem {
		return false
	}
	text := strings.ToLower(event.Text + " " + event.Detail)
	if strings.Contains(text, "git-ai") &&
		strings.Contains(text, "checkpoint") &&
		(strings.Contains(text, "falling back to local checkpoint") ||
			strings.Contains(text, "timed out waiting for trace ingest") ||
			strings.Contains(text, "checkpoint delegate")) {
		return true
	}
	return false
}

func resultFromReadOnlyTrace(event app.Event, spans []app.TraceSpan) bool {
	span := findToolSpanEndedNear(spans, event.At)
	return span != nil && isReadOnlyTool(span.Name, span.Detail)
}

func findToolSpanEndedNear(spans []app.TraceSpan, at time.Time) *app.TraceSpan {
	if at.IsZero() {
		return nil
	}
	for i := range spans {
		span := &spans[i]
		if span.Kind == app.TraceTool && !span.EndedAt.IsZero() {
			delta := span.EndedAt.Sub(at)
			if delta < 0 {
				delta = -delta
			}
			if delta <= 2*time.Second {
				return span
			}
		}
		if child := findToolSpanEndedNear(span.Children, at); child != nil {
			return child
		}
	}
	return nil
}

func resultFromReadOnlyPreviousTool(events []app.Event, index int) bool {
	for i := index - 1; i >= 0 && i >= index-4; i-- {
		if events[i].Type != app.EventTool {
			continue
		}
		return isReadOnlyTool(events[i].Text, events[i].Detail)
	}
	return false
}

func isReadOnlyTool(name string, detail string) bool {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	switch lowerName {
	case "read", "grep", "glob", "ls", "toolsearch":
		return true
	case "bash":
		return isReadOnlyCommand(detail)
	}
	if strings.HasPrefix(lowerName, "tool: ") {
		return isReadOnlyTool(strings.TrimSpace(strings.TrimPrefix(lowerName, "tool: ")), detail)
	}
	if strings.HasPrefix(lowerName, "exec: ") {
		return isReadOnlyCommand(strings.TrimSpace(strings.TrimPrefix(lowerName, "exec: ")))
	}
	return isReadOnlyCommand(detail)
}

func isReadOnlyCommand(command string) bool {
	command = strings.TrimSpace(strings.ToLower(command))
	if command == "" {
		return false
	}
	if strings.Contains(command, "go run ./cmd/cockpit inspect") ||
		(strings.Contains(command, "go run ./cmd/cockpit") && strings.Contains(command, " inspect")) {
		return true
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	first := strings.Trim(fields[0], "'\"")
	switch first {
	case "rg", "grep", "jq", "sed", "cat", "head", "tail", "ls", "find", "wc", "pwd":
		return true
	case "git":
		if len(fields) < 2 {
			return false
		}
		switch fields[1] {
		case "status", "diff", "show", "log", "grep", "ls-files":
			return true
		}
	}
	return false
}

func isExpectedFailureText(text string) bool {
	if strings.Contains(text, "unexpected") || strings.Contains(text, "not expected") {
		return false
	}
	return strings.Contains(text, "expected") ||
		strings.Contains(text, "known failure") ||
		strings.Contains(text, "intentional failure") ||
		strings.Contains(text, "预期") ||
		strings.Contains(text, "可忽略")
}

func isWaiting(events []app.Event) bool {
	event, ok := latestSignificant(events)
	if !ok || event.Type != app.EventAssistant {
		return false
	}
	return isQuestionText(event.Text)
}

func isCompleted(events []app.Event) bool {
	event, ok := latestSignificant(events)
	if !ok || event.Type != app.EventAssistant {
		return false
	}
	text := strings.ToLower(event.Text)
	return strings.Contains(text, "done") ||
		strings.Contains(text, "completed") ||
		strings.Contains(text, "finished") ||
		strings.Contains(text, "已完成") ||
		strings.Contains(text, "完成了") ||
		strings.Contains(text, "搞定") ||
		strings.Contains(text, "修好了") ||
		strings.Contains(text, "处理好了")
}

func isDecisionQuestionText(text string) bool {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	patterns := []string{
		"want me to",
		"should i",
		"shall i",
		"do you want",
		"please confirm",
		"need your",
		"requires your",
		"需要你",
		"请确认",
		"确认一下",
		"你希望",
		"你要",
		"要我",
		"要不要",
		"是否",
		"是否继续",
		"继续吗",
		"可以吗",
		"怎么处理",
		"怎么改",
		"选哪个",
		"告诉我",
		"你先告诉我",
		"状态是什么",
	}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return strings.Contains(trimmed, "？") && (strings.Contains(trimmed, "你") || strings.Contains(trimmed, "要"))
}

func isQuestionText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.HasSuffix(trimmed, "?") || strings.HasSuffix(trimmed, "？") {
		return true
	}
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, "want me to") ||
		strings.Contains(lower, "shall i") ||
		strings.Contains(lower, "should i") ||
		strings.Contains(trimmed, "吗？") ||
		strings.Contains(trimmed, "么？") ||
		strings.Contains(trimmed, "什么") ||
		strings.Contains(trimmed, "哪")
}

func latestSignificant(events []app.Event) (app.Event, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		switch events[i].Type {
		case app.EventAssistant, app.EventUser, app.EventTool, app.EventResult, app.EventError:
			return events[i], true
		}
	}
	return app.Event{}, false
}

func idleFromLatestEvent(events []app.Event, idleAfter time.Duration) (app.Status, string, bool) {
	event, ok := latestSignificant(events)
	if !ok || event.At.IsZero() {
		return "", "", false
	}
	age := time.Since(event.At)
	if age < 0 {
		return "", "", false
	}
	if event.Type == app.EventAssistant && age > 30*time.Second {
		return app.StatusIdle, fmt.Sprintf("agent response ended; idle for %s", roundDuration(age)), true
	}
	if age > idleAfter {
		return app.StatusIdle, fmt.Sprintf("idle for %s", roundDuration(age)), true
	}
	return "", "", false
}

func truncateRunes(text string, limit int) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit-1]) + "…"
}

func roundDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	return d.Round(time.Minute).String()
}
