package app

import "testing"

func TestCleanStringRemovesUnsafeControlRunes(t *testing.T) {
	got := CleanString("alpha\x00beta\x1fgamma\u0085delta\n\tok")
	want := "alpha beta gamma delta\n\tok"
	if got != want {
		t.Fatalf("CleanString() = %q, want %q", got, want)
	}
}

func TestSanitizeSnapshotCleansNestedStrings(t *testing.T) {
	snap := Snapshot{
		Tasks: []Task{
			{
				ID: "task\x00one",
				Session: Session{
					LastPrompt: "prompt\x00text",
					Events:     []Event{{Text: "event\x00text"}},
					Trace:      []TraceSpan{{ID: "trace\x00one", Summary: "summary\x00text"}},
				},
				StatusExplain: StatusExplanation{
					Reason:   "reason\x00text",
					Evidence: []string{"evidence\x00text"},
				},
				Summary: "task\x00summary",
				Flight:  []FlightEvent{{Title: "flight\x00title"}},
			},
		},
		Missions: []Mission{{Name: "mission\x00name", TaskIDs: []string{"task\x00one"}}},
		Errors:   []string{"err\x00or"},
	}

	SanitizeSnapshot(&snap)

	if snap.Tasks[0].ID != "task one" {
		t.Fatalf("task ID = %q", snap.Tasks[0].ID)
	}
	if snap.Tasks[0].Session.LastPrompt != "prompt text" {
		t.Fatalf("last prompt = %q", snap.Tasks[0].Session.LastPrompt)
	}
	if snap.Tasks[0].Session.Events[0].Text != "event text" {
		t.Fatalf("event text = %q", snap.Tasks[0].Session.Events[0].Text)
	}
	if snap.Tasks[0].Session.Trace[0].Summary != "summary text" {
		t.Fatalf("trace summary = %q", snap.Tasks[0].Session.Trace[0].Summary)
	}
	if snap.Tasks[0].StatusExplain.Evidence[0] != "evidence text" {
		t.Fatalf("evidence = %q", snap.Tasks[0].StatusExplain.Evidence[0])
	}
	if snap.Tasks[0].Flight[0].Title != "flight title" {
		t.Fatalf("flight title = %q", snap.Tasks[0].Flight[0].Title)
	}
	if snap.Missions[0].TaskIDs[0] != "task one" {
		t.Fatalf("mission task id = %q", snap.Missions[0].TaskIDs[0])
	}
	if snap.Errors[0] != "err or" {
		t.Fatalf("error = %q", snap.Errors[0])
	}
}
