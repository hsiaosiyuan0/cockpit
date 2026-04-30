package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"cockpit/internal/app"
	"cockpit/internal/config"
)

func TestParseCodexTraceKeepsOpenToolRunning(t *testing.T) {
	path := writeJSONL(t,
		`{"timestamp":"2026-04-30T07:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/tmp/repo"}}`,
		`{"timestamp":"2026-04-30T07:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"run tests"}}`,
		`{"timestamp":"2026-04-30T07:00:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"call_1","arguments":"{\"cmd\":\"go test ./...\"}"}}`,
	)

	session, err := parseCodexSession(path, config.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if !app.HasRunningWorkTrace(session.Trace) {
		t.Fatalf("expected open tool trace, got %#v", session.Trace)
	}
}

func TestParseCodexTraceKeepsExecCommandRunningAfterYieldedOutput(t *testing.T) {
	path := writeJSONL(t,
		`{"timestamp":"2026-04-30T07:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/tmp/repo"}}`,
		`{"timestamp":"2026-04-30T07:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"build app"}}`,
		`{"timestamp":"2026-04-30T07:00:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"call_1","arguments":"{\"cmd\":\"wails build\"}"}}`,
		`{"timestamp":"2026-04-30T07:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"Process running with session ID 12345"}}`,
	)

	session, err := parseCodexSession(path, config.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if !app.HasRunningWorkTrace(session.Trace) {
		t.Fatalf("expected yielded exec command to remain running, got %#v", session.Trace)
	}
}

func TestParseCodexTraceKeepsSpawnedSubagentRunningUntilWait(t *testing.T) {
	path := writeJSONL(t,
		`{"timestamp":"2026-04-30T07:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/tmp/repo"}}`,
		`{"timestamp":"2026-04-30T07:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"delegate work"}}`,
		`{"timestamp":"2026-04-30T07:00:02Z","type":"response_item","payload":{"type":"function_call","name":"spawn_agent","call_id":"spawn_1","arguments":"{\"message\":\"check UI\"}"}}`,
		`{"timestamp":"2026-04-30T07:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"spawn_1","output":"spawned agent worker-1"}}`,
	)

	session, err := parseCodexSession(path, config.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if !app.HasRunningWorkTrace(session.Trace) {
		t.Fatalf("expected spawned subagent to stay running, got %#v", session.Trace)
	}
}

func TestParseCodexTraceWaitClosesSpawnedSubagent(t *testing.T) {
	path := writeJSONL(t,
		`{"timestamp":"2026-04-30T07:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/tmp/repo"}}`,
		`{"timestamp":"2026-04-30T07:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"delegate work"}}`,
		`{"timestamp":"2026-04-30T07:00:02Z","type":"response_item","payload":{"type":"function_call","name":"spawn_agent","call_id":"spawn_1","arguments":"{\"message\":\"check UI\"}"}}`,
		`{"timestamp":"2026-04-30T07:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"spawn_1","output":"spawned agent worker-1"}}`,
		`{"timestamp":"2026-04-30T07:00:04Z","type":"response_item","payload":{"type":"function_call","name":"wait_agent","call_id":"wait_1","arguments":"{\"targets\":[\"worker-1\"]}"}}`,
		`{"timestamp":"2026-04-30T07:00:05Z","type":"response_item","payload":{"type":"function_call_output","call_id":"wait_1","output":"worker completed"}}`,
	)

	session, err := parseCodexSession(path, config.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if app.HasRunningWorkTrace(session.Trace) {
		t.Fatalf("expected wait_agent to close running subagent, got %#v", session.Trace)
	}
}

func writeJSONL(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rollout-2026-04-30T00-00-00-s1.jsonl")
	data := ""
	for _, line := range lines {
		data += line + "\n"
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
