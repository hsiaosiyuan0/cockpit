package discovery

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"cockpit/internal/app"
	"cockpit/internal/config"
)

var codexPIDPattern = regexp.MustCompile(`^pid:(\d+):`)

func DiscoverCodex(ctx context.Context, cfg config.Config) ([]app.Session, error) {
	paths, err := recentFiles(filepath.Join(cfg.CodexHome, "sessions"), "*.jsonl", cfg.RecentWindow)
	if err != nil {
		return nil, err
	}
	sessions := make([]app.Session, 0, len(paths))
	for _, path := range paths {
		session, err := parseCodexSession(path, cfg)
		if err != nil || session.ID == "" {
			continue
		}
		session.Agent = app.AgentCodex
		session.LogPath = path
		markInternal(&session, cfg)
		sessions = append(sessions, session)
	}
	enrichCodexPIDs(ctx, cfg, sessions)
	return sessions, nil
}

func parseCodexSession(path string, cfg config.Config) (app.Session, error) {
	var session app.Session
	trace := newTraceBuilder()
	err := scanJSONL(path, func(obj map[string]any) {
		ts := timeField(obj, "timestamp")
		switch stringField(obj, "type") {
		case "session_meta":
			payload := mapField(obj, "payload")
			if payload == nil {
				return
			}
			session.ID = stringField(payload, "id")
			session.CWD = stringField(payload, "cwd")
			addEvent(&session, app.Event{At: ts, Type: app.EventSystem, Text: "session started"})
		case "event_msg":
			payload := mapField(obj, "payload")
			if payload == nil {
				return
			}
			parseCodexEventPayload(&session, trace, ts, payload)
		case "response_item":
			payload := mapField(obj, "payload")
			if payload == nil {
				return
			}
			parseCodexResponseItem(&session, trace, ts, payload)
		}
	})
	if session.ID == "" {
		session.ID = idFromCodexFilename(path)
	}
	session.Events = lastEvents(session.Events, 80)
	session.Trace = trace.finalize()
	return session, err
}

func parseCodexEventPayload(session *app.Session, trace *traceBuilder, ts time.Time, payload map[string]any) {
	switch stringField(payload, "type") {
	case "user_message":
		text := stringField(payload, "message")
		session.LastPrompt = text
		trace.startTurn(ts, text)
		addEvent(session, app.Event{At: ts, Type: app.EventUser, Text: text})
	case "agent_message":
		text := stringField(payload, "message")
		trace.addAssistant(ts, text)
		addEvent(session, app.Event{At: ts, Type: app.EventAssistant, Text: text})
	case "exec_command_begin":
		callID := stringField(payload, "call_id")
		detail := commandSummary(payload)
		if callID != "" {
			trace.startTool(callID, "exec_command", app.TraceTool, ts, detail)
		}
		addEvent(session, app.Event{At: ts, Type: app.EventTool, Text: "exec: " + detail})
	case "exec_command_end":
		text := stringField(payload, "stdout")
		if text == "" {
			text = stringField(payload, "stderr")
		}
		if text == "" {
			text = stringField(payload, "aggregated_output")
		}
		if text == "" {
			text = "command completed"
		}
		trace.finishTool(stringField(payload, "call_id"), ts, intField(payload, "exit_code"), text)
		addEvent(session, app.Event{At: ts, Type: app.EventResult, Text: text})
	case "patch_apply_begin":
		if callID := stringField(payload, "call_id"); callID != "" {
			trace.startTool(callID, "apply_patch", app.TraceTool, ts, "patch")
		}
		addEvent(session, app.Event{At: ts, Type: app.EventTool, Text: stringField(payload, "type")})
	case "patch_apply_end":
		if callID := stringField(payload, "call_id"); callID != "" {
			trace.finishTool(callID, ts, nil, "patch applied")
		}
		addEvent(session, app.Event{At: ts, Type: app.EventTool, Text: stringField(payload, "type")})
	case "error":
		addEvent(session, app.Event{At: ts, Type: app.EventError, Text: stringField(payload, "message")})
	default:
		if msg := stringField(payload, "message"); msg != "" {
			addEvent(session, app.Event{At: ts, Type: app.EventSystem, Text: msg, Detail: stringField(payload, "type")})
		}
	}
}

func parseCodexResponseItem(session *app.Session, trace *traceBuilder, ts time.Time, payload map[string]any) {
	switch stringField(payload, "type") {
	case "message":
		role := stringField(payload, "role")
		text := responseText(payload)
		if text == "" {
			return
		}
		eventType := app.EventAssistant
		if role == "user" {
			eventType = app.EventUser
			session.LastPrompt = text
			trace.startTurn(ts, text)
		} else {
			trace.addAssistant(ts, text)
		}
		addEvent(session, app.Event{At: ts, Type: eventType, Text: text})
	case "function_call":
		name := stringField(payload, "name")
		if name == "" {
			name = stringField(payload, "tool_name")
		}
		trace.startTool(stringField(payload, "call_id"), name, traceKindForTool(name), ts, traceDetailForTool(name, payload))
		addEvent(session, app.Event{At: ts, Type: app.EventTool, Text: "tool: " + name})
	case "function_call_output":
		output := stringField(payload, "output")
		callID := stringField(payload, "call_id")
		trace.finishTool(callID, ts, outputExitCode(output), output)
		if span := trace.spans[callID]; span != nil && span.Kind == app.TraceSubagent && (span.Name == "wait_agent" || span.Name == "close_agent") {
			trace.finishRunningSubagents(ts, output)
		}
		addEvent(session, app.Event{At: ts, Type: app.EventResult, Text: output})
	}
}

func responseText(payload map[string]any) string {
	var parts []string
	for _, item := range sliceField(payload, "content") {
		content, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if text := stringField(content, "text"); text != "" {
			parts = append(parts, text)
			continue
		}
		if text := stringField(content, "output_text"); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func enrichCodexPIDs(ctx context.Context, cfg config.Config, sessions []app.Session) {
	dbPath := filepath.Join(cfg.CodexHome, "logs_2.sqlite")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(1000)")
	if err != nil {
		return
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `select thread_id, process_uuid, max(ts) from logs where thread_id is not null group by thread_id, process_uuid`)
	if err != nil {
		return
	}
	defer rows.Close()
	byID := map[string]*app.Session{}
	for i := range sessions {
		byID[sessions[i].ID] = &sessions[i]
	}
	for rows.Next() {
		var threadID, processUUID string
		var ts int64
		if err := rows.Scan(&threadID, &processUUID, &ts); err != nil {
			continue
		}
		session := byID[threadID]
		if session == nil {
			continue
		}
		if session.ProcessUUID == "" || unixTime(ts).After(session.LastEventAt.Add(-5*time.Second)) {
			session.ProcessUUID = processUUID
			if pid := pidFromProcessUUID(processUUID); pid != 0 {
				session.PID = pid
			}
		}
	}
}

func pidFromProcessUUID(processUUID string) int {
	match := codexPIDPattern.FindStringSubmatch(processUUID)
	if len(match) != 2 {
		return 0
	}
	var pid int
	_, _ = fmt.Sscanf(match[1], "%d", &pid)
	return pid
}

func idFromCodexFilename(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	idx := strings.LastIndex(base, "-")
	if idx < 0 || idx+1 >= len(base) {
		return base
	}
	return base[idx+1:]
}

func recentFiles(root, pattern string, window time.Duration) ([]string, error) {
	cutoff := time.Now().Add(-window)
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err != nil || !matched {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(cutoff) {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Slice(paths, func(i, j int) bool {
		ii, _ := os.Stat(paths[i])
		jj, _ := os.Stat(paths[j])
		if ii == nil || jj == nil {
			return paths[i] > paths[j]
		}
		return ii.ModTime().After(jj.ModTime())
	})
	return paths, err
}

func unixTime(ts int64) time.Time {
	if ts <= 0 {
		return time.Time{}
	}
	return time.Unix(ts, 0)
}
