package discovery

import (
	"testing"

	"cockpit/internal/app"
	"cockpit/internal/config"
)

func TestParseClaudeTraceKeepsOpenToolUseRunning(t *testing.T) {
	path := writeJSONL(t,
		`{"timestamp":"2026-04-30T07:00:00Z","type":"user","sessionId":"c1","cwd":"/tmp/repo","message":{"content":[{"type":"text","text":"run tests"}]}}`,
		`{"timestamp":"2026-04-30T07:00:01Z","type":"assistant","sessionId":"c1","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"go test ./..."}}]}}`,
	)

	session, err := parseClaudeSession(path, config.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if !app.HasRunningWorkTrace(session.Trace) {
		t.Fatalf("expected open Claude tool_use trace, got %#v", session.Trace)
	}
}

func TestParseClaudeTraceToolResultClosesToolUse(t *testing.T) {
	path := writeJSONL(t,
		`{"timestamp":"2026-04-30T07:00:00Z","type":"user","sessionId":"c1","cwd":"/tmp/repo","message":{"content":[{"type":"text","text":"run tests"}]}}`,
		`{"timestamp":"2026-04-30T07:00:01Z","type":"assistant","sessionId":"c1","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"go test ./..."}}]}}`,
		`{"timestamp":"2026-04-30T07:00:02Z","type":"user","sessionId":"c1","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}}`,
	)

	session, err := parseClaudeSession(path, config.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if app.HasRunningWorkTrace(session.Trace) {
		t.Fatalf("expected Claude tool_result to close trace, got %#v", session.Trace)
	}
}
