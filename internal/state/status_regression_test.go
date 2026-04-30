package state

import (
	"testing"
	"time"

	"cockpit/internal/app"
)

func TestStatusExplanationRegressionFixtures(t *testing.T) {
	now := time.Now()
	fixtures := []struct {
		name       string
		task       app.Task
		wantStatus app.Status
		wantRule   string
		wantActive bool
		wantBg     int
	}{
		{
			name: "codex idle with yielded background dev server",
			task: regressionTask(now, []app.Event{
				{At: now.Add(-90 * time.Second), Type: app.EventAssistant, Text: "dev server is running; ready for review"},
			}, []app.TraceSpan{
				{
					ID:        "turn:1",
					Kind:      app.TraceTurn,
					Name:      "user turn",
					Status:    app.TraceRunning,
					StartedAt: now.Add(-5 * time.Minute),
					Children: []app.TraceSpan{
						{ID: "call:dev", ParentID: "turn:1", Kind: app.TraceTool, Name: "exec_command", Status: app.TraceRunning, StartedAt: now.Add(-4 * time.Minute), Detail: "PORT=8790 npm run dev"},
						{ID: "assistant:1", ParentID: "turn:1", Kind: app.TraceAssistant, Name: "assistant", Status: app.TraceSucceeded, StartedAt: now.Add(-90 * time.Second), EndedAt: now.Add(-90 * time.Second), Summary: "dev server is running; ready for review"},
					},
				},
			}),
			wantStatus: app.StatusIdle,
			wantRule:   "idle_timeout",
			wantBg:     1,
		},
		{
			name: "subagent still running after assistant response",
			task: regressionTask(now, []app.Event{
				{At: now.Add(-90 * time.Second), Type: app.EventAssistant, Text: "worker is still checking this in the background"},
			}, []app.TraceSpan{
				{ID: "subagent:1", Kind: app.TraceSubagent, Name: "spawn_agent", Status: app.TraceRunning, StartedAt: now.Add(-4 * time.Minute), Detail: "worker"},
			}),
			wantStatus: app.StatusWorking,
			wantRule:   "trace_active",
			wantActive: true,
		},
		{
			name: "expected localhost error is not attention",
			task: regressionTask(now, []app.Event{
				{At: now.Add(-2 * time.Minute), Type: app.EventResult, Text: "curl: (22) The requested URL returned error: 502 (localhost test fails - expected; no caddy running)"},
				{At: now.Add(-90 * time.Second), Type: app.EventAssistant, Text: "localhost failure is expected on this machine"},
			}, nil),
			wantStatus: app.StatusIdle,
			wantRule:   "idle_timeout",
		},
		{
			name: "unexpected command error needs attention",
			task: regressionTask(now, []app.Event{
				{At: now.Add(-2 * time.Minute), Type: app.EventResult, Text: "curl: (22) The requested URL returned error: 502 unexpected"},
			}, nil),
			wantStatus: app.StatusNeedsAttention,
			wantRule:   "attention_pattern",
		},
		{
			name: "assistant asks prod config decision after successful diagnostics",
			task: regressionTask(now, []app.Event{
				{At: now.Add(-70 * time.Second), Type: app.EventResult, Text: `ok {"timestamp":"2026-04-30 17:48:14","status":404,"error":"Not Found","path":"/actuator/health"}`},
				{At: now.Add(-40 * time.Second), Type: app.EventAssistant, Text: "prod cluster 上 rds namespace 可能漏了。要不要我把 opconfig 检查清单整理出来？"},
			}, nil),
			wantStatus: app.StatusNeedsAttention,
			wantRule:   "attention_pattern",
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			explanation := Explain(fixture.task, 10*time.Minute)
			if explanation.Status != fixture.wantStatus {
				t.Fatalf("status = %s, want %s; explanation=%#v", explanation.Status, fixture.wantStatus, explanation)
			}
			if explanation.Rule != fixture.wantRule {
				t.Fatalf("rule = %q, want %q; explanation=%#v", explanation.Rule, fixture.wantRule, explanation)
			}
			if (explanation.ActiveTrace != nil) != fixture.wantActive {
				t.Fatalf("active trace present = %v, want %v; explanation=%#v", explanation.ActiveTrace != nil, fixture.wantActive, explanation)
			}
			if len(explanation.BackgroundRunTraces) != fixture.wantBg {
				t.Fatalf("background traces = %d, want %d; explanation=%#v", len(explanation.BackgroundRunTraces), fixture.wantBg, explanation)
			}
		})
	}
}

func regressionTask(now time.Time, events []app.Event, trace []app.TraceSpan) app.Task {
	task := boundTask(events)
	task.Session.CWD = "/Users/hsy/dev/cockpit"
	task.Session.Trace = trace
	if len(events) > 0 {
		task.Session.LastEventAt = events[len(events)-1].At
	} else {
		task.Session.LastEventAt = now
	}
	return task
}
