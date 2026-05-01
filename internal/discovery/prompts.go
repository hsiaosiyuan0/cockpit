package discovery

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cockpit/internal/app"
	"cockpit/internal/config"
)

func DiscoverPromptSessions(ctx context.Context, cfg config.Config) ([]app.Session, error) {
	_ = ctx
	var errs []error
	var sessions []app.Session

	codexSessions, err := discoverCodexPromptSessions(cfg)
	if err != nil {
		errs = append(errs, err)
	}
	sessions = append(sessions, codexSessions...)

	claudeSessions, err := discoverClaudePromptSessions(cfg)
	if err != nil {
		errs = append(errs, err)
	}
	sessions = append(sessions, claudeSessions...)
	sessions = dedupeSessions(sessions)

	cutoff := time.Now().Add(-cfg.RecentWindow)
	filtered := sessions[:0]
	for _, session := range sessions {
		if session.LastEventAt.IsZero() || session.LastEventAt.After(cutoff) || session.PID != 0 {
			filtered = append(filtered, session)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].LastEventAt.After(filtered[j].LastEventAt)
	})
	if len(filtered) > cfg.MaxSessions {
		filtered = filtered[:cfg.MaxSessions]
	}
	return filtered, errors.Join(errs...)
}

func discoverCodexPromptSessions(cfg config.Config) ([]app.Session, error) {
	paths, err := recentFiles(filepath.Join(cfg.CodexHome, "sessions"), "*.jsonl", cfg.RecentWindow)
	if err != nil {
		return nil, err
	}
	sessions := make([]app.Session, 0, len(paths))
	var errs []error
	for _, path := range paths {
		session, parseErr := parseCodexPromptSession(path)
		if parseErr != nil {
			errs = append(errs, fmt.Errorf("codex prompt scan skipped %s: %w", path, parseErr))
			continue
		}
		if session.ID == "" {
			continue
		}
		session.Agent = app.AgentCodex
		session.LogPath = path
		markInternal(&session, cfg)
		sessions = append(sessions, session)
	}
	return sessions, errors.Join(errs...)
}

func discoverClaudePromptSessions(cfg config.Config) ([]app.Session, error) {
	paths, err := recentFiles(filepath.Join(cfg.ClaudeHome, "projects"), "*.jsonl", cfg.RecentWindow)
	if err != nil {
		return nil, err
	}
	sessions := make([]app.Session, 0, len(paths))
	var errs []error
	for _, path := range paths {
		session, parseErr := parseClaudePromptSession(path)
		if parseErr != nil {
			errs = append(errs, fmt.Errorf("claude prompt scan skipped %s: %w", path, parseErr))
			continue
		}
		if session.ID == "" {
			continue
		}
		session.Agent = app.AgentClaude
		session.LogPath = path
		markInternal(&session, cfg)
		sessions = append(sessions, session)
	}
	return sessions, errors.Join(errs...)
}

func parseCodexPromptSession(path string) (app.Session, error) {
	var session app.Session
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
		case "event_msg":
			payload := mapField(obj, "payload")
			if payload == nil || stringField(payload, "type") != "user_message" {
				return
			}
			addPromptEvent(&session, ts, stringField(payload, "message"))
		case "response_item":
			payload := mapField(obj, "payload")
			if payload == nil || stringField(payload, "type") != "message" || stringField(payload, "role") != "user" {
				return
			}
			addPromptEvent(&session, ts, responseText(payload))
		}
	})
	if session.ID == "" {
		session.ID = idFromCodexFilename(path)
	}
	return session, err
}

func parseClaudePromptSession(path string) (app.Session, error) {
	var session app.Session
	err := scanJSONL(path, func(obj map[string]any) {
		if sid := stringField(obj, "sessionId"); sid != "" {
			session.ID = sid
		}
		if cwd := stringField(obj, "cwd"); cwd != "" {
			session.CWD = cwd
		}
		if stringField(obj, "type") != "user" {
			return
		}
		message := mapField(obj, "message")
		if isToolResult(message) {
			return
		}
		addPromptEvent(&session, timeField(obj, "timestamp"), claudeMessageText(message))
	})
	if session.ID == "" {
		session.ID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	return session, err
}

func addPromptEvent(session *app.Session, ts time.Time, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	session.LastPrompt = text
	addEvent(session, app.Event{At: ts, Type: app.EventUser, Text: text})
}
