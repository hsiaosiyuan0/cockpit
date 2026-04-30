package discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cockpit/internal/app"
	"cockpit/internal/config"
)

type claudeLiveSession struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
}

func DiscoverClaude(ctx context.Context, cfg config.Config) ([]app.Session, error) {
	_ = ctx
	paths, err := recentFiles(filepath.Join(cfg.ClaudeHome, "projects"), "*.jsonl", cfg.RecentWindow)
	if err != nil {
		return nil, err
	}
	sessions := make([]app.Session, 0, len(paths))
	for _, path := range paths {
		session, err := parseClaudeSession(path, cfg)
		if err != nil || session.ID == "" {
			continue
		}
		session.Agent = app.AgentClaude
		session.LogPath = path
		markInternal(&session, cfg)
		sessions = append(sessions, session)
	}
	live := loadClaudeLiveSessions(cfg)
	for i := range sessions {
		if liveSession, ok := live[sessions[i].ID]; ok {
			sessions[i].PID = liveSession.PID
			if sessions[i].CWD == "" {
				sessions[i].CWD = liveSession.CWD
			}
		}
	}
	return sessions, nil
}

func parseClaudeSession(path string, cfg config.Config) (app.Session, error) {
	var session app.Session
	trace := newTraceBuilder()
	err := scanJSONL(path, func(obj map[string]any) {
		if sid := stringField(obj, "sessionId"); sid != "" {
			session.ID = sid
		}
		if cwd := stringField(obj, "cwd"); cwd != "" {
			session.CWD = cwd
		}
		ts := timeField(obj, "timestamp")
		switch stringField(obj, "type") {
		case "user":
			text := claudeMessageText(mapField(obj, "message"))
			if isToolResult(mapField(obj, "message")) {
				parseClaudeToolResults(trace, ts, mapField(obj, "message"))
				addEvent(&session, app.Event{At: ts, Type: app.EventResult, Text: text})
			} else {
				session.LastPrompt = text
				trace.startTurn(ts, text)
				addEvent(&session, app.Event{At: ts, Type: app.EventUser, Text: text})
			}
		case "assistant":
			parseClaudeAssistant(&session, trace, ts, mapField(obj, "message"))
		case "system":
			subtype := stringField(obj, "subtype")
			if subtype == "away_summary" {
				addEvent(&session, app.Event{At: ts, Type: app.EventSystem, Text: stringField(obj, "content"), Detail: subtype})
			} else if subtype == "turn_duration" {
				trace.finishCurrentTurn(ts)
				addEvent(&session, app.Event{At: ts, Type: app.EventSystem, Text: "turn completed", Detail: subtype})
			}
		case "attachment":
			attachment := mapField(obj, "attachment")
			if attachment != nil {
				addEvent(&session, app.Event{At: ts, Type: app.EventSystem, Text: stringField(attachment, "hookName"), Detail: stringField(attachment, "stderr")})
			}
		case "last-prompt":
			session.LastPrompt = stringField(obj, "lastPrompt")
		}
	})
	if session.ID == "" {
		session.ID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	session.Events = lastEvents(session.Events, 80)
	session.Trace = trace.finalize()
	return session, err
}

func parseClaudeAssistant(session *app.Session, trace *traceBuilder, ts time.Time, message map[string]any) {
	if message == nil {
		return
	}
	var textParts []string
	for _, item := range sliceField(message, "content") {
		content, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch stringField(content, "type") {
		case "text":
			textParts = append(textParts, stringField(content, "text"))
		case "tool_use":
			id := stringField(content, "id")
			name := stringField(content, "name")
			input := mapField(content, "input")
			detail := ""
			if input != nil {
				detail = stringField(input, "command")
				if detail == "" {
					detail = stringField(input, "file_path")
				}
			}
			trace.startTool("claude:"+id, name, traceKindForTool(name), ts, detail)
			addEvent(session, app.Event{At: ts, Type: app.EventTool, Text: "tool: " + name, Detail: detail})
		}
	}
	if len(textParts) > 0 {
		text := strings.Join(textParts, "\n")
		trace.addAssistant(ts, text)
		addEvent(session, app.Event{At: ts, Type: app.EventAssistant, Text: text})
	}
}

func parseClaudeToolResults(trace *traceBuilder, ts time.Time, message map[string]any) {
	for _, item := range sliceField(message, "content") {
		content, ok := item.(map[string]any)
		if !ok || stringField(content, "type") != "tool_result" {
			continue
		}
		id := stringField(content, "tool_use_id")
		output := stringField(content, "content")
		trace.finishTool("claude:"+id, ts, nil, output)
		if span := trace.spans["claude:"+id]; span != nil && span.Kind == app.TraceSubagent {
			trace.finishRunningSubagents(ts, output)
		}
	}
}

func claudeMessageText(message map[string]any) string {
	if message == nil {
		return ""
	}
	var parts []string
	for _, item := range sliceField(message, "content") {
		switch content := item.(type) {
		case string:
			parts = append(parts, content)
		case map[string]any:
			if text := stringField(content, "text"); text != "" {
				parts = append(parts, text)
			}
			if text := stringField(content, "content"); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func isToolResult(message map[string]any) bool {
	for _, item := range sliceField(message, "content") {
		content, ok := item.(map[string]any)
		if ok && stringField(content, "type") == "tool_result" {
			return true
		}
	}
	return false
}

func loadClaudeLiveSessions(cfg config.Config) map[string]claudeLiveSession {
	out := map[string]claudeLiveSession{}
	paths, _ := filepath.Glob(filepath.Join(cfg.ClaudeHome, "sessions", "*.json"))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var live claudeLiveSession
		if err := json.Unmarshal(data, &live); err != nil {
			continue
		}
		if live.PID == 0 {
			base := strings.TrimSuffix(filepath.Base(path), ".json")
			live.PID, _ = strconv.Atoi(base)
		}
		if live.SessionID != "" {
			out[live.SessionID] = live
		}
	}
	return out
}
