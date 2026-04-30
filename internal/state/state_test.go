package state

import (
	"testing"
	"time"

	"cockpit/internal/app"
)

func TestEvaluateAssistantResponseSettlesToIdleBeforeGlobalIdleThreshold(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-2 * time.Minute), Type: app.EventAssistant, Text: "updated the files and ran the checks"},
	})

	status, reason := Evaluate(task, 10*time.Minute)

	if status != app.StatusIdle {
		t.Fatalf("status = %s, want %s", status, app.StatusIdle)
	}
	if reason == "" {
		t.Fatal("expected idle reason")
	}
}

func TestEvaluateKeepsOpenToolTraceWorkingDespiteOldAssistantMessage(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-2 * time.Minute), Type: app.EventAssistant, Text: "I will keep checking the implementation"},
	})
	task.Session.Trace = []app.TraceSpan{
		{
			ID:        "turn:1",
			Kind:      app.TraceTurn,
			Name:      "user turn",
			Status:    app.TraceRunning,
			StartedAt: now.Add(-3 * time.Minute),
			Children: []app.TraceSpan{
				{
					ID:        "call:1",
					ParentID:  "turn:1",
					Kind:      app.TraceTool,
					Name:      "exec_command",
					Status:    app.TraceRunning,
					StartedAt: now.Add(-90 * time.Second),
				},
			},
		},
	}

	status, reason := Evaluate(task, 10*time.Minute)

	if status != app.StatusWorking {
		t.Fatalf("status = %s, want %s", status, app.StatusWorking)
	}
	if reason == "" {
		t.Fatal("expected running trace reason")
	}
}

func TestEvaluateKeepsOpenSubagentTraceWorkingDespiteOldAssistantMessage(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-2 * time.Minute), Type: app.EventAssistant, Text: "I spawned a worker to continue"},
	})
	task.Session.Trace = []app.TraceSpan{
		{
			ID:        "subagent:1",
			Kind:      app.TraceSubagent,
			Name:      "spawn_agent",
			Status:    app.TraceRunning,
			StartedAt: now.Add(-90 * time.Second),
		},
	}

	status, _ := Evaluate(task, 10*time.Minute)

	if status != app.StatusWorking {
		t.Fatalf("status = %s, want %s", status, app.StatusWorking)
	}
}

func TestEvaluateTreatsOpenToolAsBackgroundWhenAssistantRespondedAfterStart(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-1 * time.Minute), Type: app.EventAssistant, Text: "dev server is running; ready for review"},
	})
	task.Session.Trace = []app.TraceSpan{
		{
			ID:        "call:server",
			Kind:      app.TraceTool,
			Name:      "exec_command",
			Status:    app.TraceRunning,
			StartedAt: now.Add(-3 * time.Minute),
			Detail:    "npm run dev",
		},
	}

	status, reason := Evaluate(task, 10*time.Minute)

	if status == app.StatusWorking {
		t.Fatalf("status = %s reason=%q, want non-working background tool", status, reason)
	}
	explanation := Explain(task, 10*time.Minute)
	if explanation.WhyNotWorking == "" || len(explanation.BackgroundRunTraces) != 1 {
		t.Fatalf("explanation = %#v, want background tool and non-working reason", explanation)
	}
}

func TestEvaluateTreatsOpenToolAsBackgroundWhenTraceAssistantRespondedAfterStart(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-11 * time.Minute), Type: app.EventResult, Text: "process yielded"},
	})
	task.Session.Trace = []app.TraceSpan{
		{
			ID:        "turn:1",
			Kind:      app.TraceTurn,
			Name:      "user turn",
			Status:    app.TraceRunning,
			StartedAt: now.Add(-4 * time.Minute),
			Children: []app.TraceSpan{
				{
					ID:        "call:server",
					ParentID:  "turn:1",
					Kind:      app.TraceTool,
					Name:      "exec_command",
					Status:    app.TraceRunning,
					StartedAt: now.Add(-3 * time.Minute),
					Detail:    "npm run dev",
				},
				{
					ID:        "assistant:1",
					ParentID:  "turn:1",
					Kind:      app.TraceAssistant,
					Name:      "assistant",
					Status:    app.TraceSucceeded,
					StartedAt: now.Add(-1 * time.Minute),
					EndedAt:   now.Add(-1 * time.Minute),
					Summary:   "dev server is running; ready for review",
				},
			},
		},
	}

	status, reason := Evaluate(task, 10*time.Minute)

	if status == app.StatusWorking {
		t.Fatalf("status = %s reason=%q, want non-working background tool", status, reason)
	}
}

func TestEvaluateKeepsOpenSubagentWorkingEvenWhenAssistantRespondedAfterStart(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-1 * time.Minute), Type: app.EventAssistant, Text: "worker is still checking this in the background"},
	})
	task.Session.Trace = []app.TraceSpan{
		{
			ID:        "subagent:1",
			Kind:      app.TraceSubagent,
			Name:      "spawn_agent",
			Status:    app.TraceRunning,
			StartedAt: now.Add(-3 * time.Minute),
		},
	}

	status, _ := Evaluate(task, 10*time.Minute)

	if status != app.StatusWorking {
		t.Fatalf("status = %s, want %s", status, app.StatusWorking)
	}
}

func TestEvaluateIgnoresExpectedErrorOutput(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-2 * time.Minute), Type: app.EventResult, Text: "curl: (22) The requested URL returned error: 502 (localhost test fails - expected; no caddy running)"},
		{At: now.Add(-90 * time.Second), Type: app.EventAssistant, Text: "localhost failure is expected on this machine"},
	})

	status, reason := Evaluate(task, 10*time.Minute)

	if status == app.StatusNeedsAttention {
		t.Fatalf("status = %s reason=%q, want non-attention", status, reason)
	}
}

func TestEvaluateKeepsUnexpectedErrorOutputAttention(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-2 * time.Minute), Type: app.EventResult, Text: "curl: (22) The requested URL returned error: 502 unexpected"},
	})

	status, reason := Evaluate(task, 10*time.Minute)

	if status != app.StatusNeedsAttention || reason != "error detected" {
		t.Fatalf("status = %s reason=%q, want needs_attention error detected", status, reason)
	}
}

func TestEvaluateIgnoresGitAICheckpointSystemError(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{
			At:     now.Add(-2 * time.Minute),
			Type:   app.EventSystem,
			Text:   "PostToolUse:Bash",
			Detail: "[git-ai] checkpoint delegate request_rejected: Generic error: timed out waiting for trace ingest through seq 26902 (processed=26885); falling back to local checkpoint Checkpoint completed in 5.177715208s",
		},
		{At: now.Add(-90 * time.Second), Type: app.EventAssistant, Text: "waiting for the baseline before continuing"},
	})

	status, reason := Evaluate(task, 10*time.Minute)

	if status == app.StatusNeedsAttention {
		t.Fatalf("status = %s reason=%q, want non-attention checkpoint fallback", status, reason)
	}
}

func TestEvaluateIgnoresReadOnlySearchOutputWithErrorText(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-2 * time.Minute), Type: app.EventTool, Text: `exec: rg -n "error detected|unexpected" internal/state`},
		{At: now.Add(-2*time.Minute + time.Second), Type: app.EventResult, Text: `internal/state/state.go: return "error detected"`},
	})

	status, reason := Evaluate(task, 10*time.Minute)

	if status == app.StatusNeedsAttention {
		t.Fatalf("status = %s reason=%q, want non-attention read-only search output", status, reason)
	}
}

func TestEvaluateIgnoresReadOnlyBashSearchOutputWithErrorText(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-2 * time.Minute), Type: app.EventTool, Text: `tool: Bash`, Detail: `grep -rn "panic:" internal/state`},
		{At: now.Add(-2*time.Minute + time.Second), Type: app.EventResult, Text: `internal/state/state.go: strings.Contains(text, "panic:")`},
	})

	status, reason := Evaluate(task, 10*time.Minute)

	if status == app.StatusNeedsAttention {
		t.Fatalf("status = %s reason=%q, want non-attention read-only bash output", status, reason)
	}
}

func TestEvaluateIgnoresSuccessfulToolWrapperOutputWithErrorText(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{
			At:   now.Add(-2 * time.Minute),
			Type: app.EventResult,
			Text: "Chunk ID: abc Wall time: 0.0000 seconds Process exited with code 0 Output: Generic error: timed out waiting for trace ingest",
		},
	})

	status, reason := Evaluate(task, 10*time.Minute)

	if status == app.StatusNeedsAttention {
		t.Fatalf("status = %s reason=%q, want non-attention successful wrapper output", status, reason)
	}
}

func TestEvaluateIgnoresAssistantMentioningGenericError(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-2 * time.Minute), Type: app.EventAssistant, Text: "Generic error: timed out waiting for trace ingest is a benign checkpoint fallback"},
	})

	status, reason := Evaluate(task, 10*time.Minute)

	if status == app.StatusNeedsAttention {
		t.Fatalf("status = %s reason=%q, want non-attention assistant commentary", status, reason)
	}
}

func TestEvaluateKeepsFreshAssistantMessageWorkingDuringSettleWindow(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-5 * time.Second), Type: app.EventAssistant, Text: "checking the implementation now"},
	})

	status, _ := Evaluate(task, 10*time.Minute)

	if status != app.StatusWorking {
		t.Fatalf("status = %s, want %s", status, app.StatusWorking)
	}
}

func TestEvaluateChineseAssistantDecisionQuestionNeedsAttention(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-2 * time.Minute), Type: app.EventAssistant, Text: "你 NCM 配完之后 pod 重启了吗？现在状态是什么？"},
	})

	status, reason := Evaluate(task, 10*time.Minute)

	if status != app.StatusNeedsAttention || reason != "agent is asking for a decision" {
		t.Fatalf("status = %s reason=%q, want needs_attention decision", status, reason)
	}
	explanation := Explain(task, 10*time.Minute)
	if explanation.AttentionEvent == nil || explanation.AttentionEvent.Text == "" {
		t.Fatalf("explanation = %#v, want attention source event", explanation)
	}
}

func TestEvaluateChineseCompletionPhraseCompleted(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-2 * time.Minute), Type: app.EventAssistant, Text: "已经处理好了，测试也通过了。"},
	})

	status, _ := Evaluate(task, 10*time.Minute)

	if status != app.StatusCompleted {
		t.Fatalf("status = %s, want %s", status, app.StatusCompleted)
	}
}

func TestEvaluateKeepsRecentToolResultWorking(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-45 * time.Second), Type: app.EventResult, Text: "command completed"},
	})

	status, _ := Evaluate(task, 10*time.Minute)

	if status != app.StatusWorking {
		t.Fatalf("status = %s, want %s", status, app.StatusWorking)
	}
}

func TestEvaluateMarksOldToolResultIdleAtGlobalThreshold(t *testing.T) {
	now := time.Now()
	task := boundTask([]app.Event{
		{At: now.Add(-11 * time.Minute), Type: app.EventResult, Text: "command completed"},
	})

	status, _ := Evaluate(task, 10*time.Minute)

	if status != app.StatusIdle {
		t.Fatalf("status = %s, want %s", status, app.StatusIdle)
	}
}

func boundTask(events []app.Event) app.Task {
	last := time.Time{}
	if len(events) > 0 {
		last = events[len(events)-1].At
	}
	return app.Task{
		Session: app.Session{
			ID:          "test",
			Agent:       app.AgentCodex,
			LastEventAt: last,
			Events:      events,
		},
		Binding: app.Binding{
			Pane: &app.Pane{PaneID: 1},
		},
	}
}
