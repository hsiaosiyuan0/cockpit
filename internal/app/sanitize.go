package app

import "strings"

// CleanString removes control bytes that can break the Wails JSON bridge while
// preserving normal text, tabs, and newlines for markdown-like fields.
func CleanString(text string) string {
	if text == "" {
		return ""
	}
	valid := strings.ToValidUTF8(text, "")
	needsClean := valid != text
	if !needsClean {
		for _, r := range valid {
			if unsafeControlRune(r) {
				needsClean = true
				break
			}
		}
	}
	if !needsClean {
		return text
	}
	var b strings.Builder
	b.Grow(len(valid))
	replaced := false
	for _, r := range valid {
		if unsafeControlRune(r) {
			if !replaced {
				b.WriteByte(' ')
				replaced = true
			}
			continue
		}
		b.WriteRune(r)
		replaced = false
	}
	return b.String()
}

func unsafeControlRune(r rune) bool {
	if r == '\n' || r == '\r' || r == '\t' {
		return false
	}
	return (r >= 0 && r < 0x20) || (r >= 0x7f && r <= 0x9f)
}

// SanitizeSnapshot cleans every user/log-derived string before the snapshot is
// sent to a browser or persisted into derived state.
func SanitizeSnapshot(snap *Snapshot) {
	if snap == nil {
		return
	}
	for i := range snap.Tasks {
		SanitizeTask(&snap.Tasks[i])
	}
	for i := range snap.Panes {
		sanitizePane(&snap.Panes[i])
	}
	for i := range snap.Processes {
		sanitizeProcess(&snap.Processes[i])
	}
	for i := range snap.Missions {
		sanitizeMission(&snap.Missions[i])
	}
	for i := range snap.DoneInbox {
		sanitizeDoneInboxItem(&snap.DoneInbox[i])
	}
	for i := range snap.Errors {
		snap.Errors[i] = CleanString(snap.Errors[i])
	}
}

// SanitizeTask cleans user/log-derived task fields in-place.
func SanitizeTask(task *Task) {
	if task == nil {
		return
	}
	task.ID = CleanString(task.ID)
	sanitizeSession(&task.Session)
	sanitizeBinding(&task.Binding)
	sanitizeStatusExplanation(&task.StatusExplain)
	task.AttentionReason = CleanString(task.AttentionReason)
	task.Summary = CleanString(task.Summary)
	task.InputAlertReason = CleanString(task.InputAlertReason)
	sanitizeDiffRadar(&task.Diff)
	for i := range task.Flight {
		sanitizeFlightEvent(&task.Flight[i])
	}
	if task.Debrief != nil {
		sanitizeDebrief(task.Debrief)
	}
}

// SanitizePromptSearchResults cleans search results built from historical logs.
func SanitizePromptSearchResults(results []PromptSearchResult) {
	for i := range results {
		results[i].ID = CleanString(results[i].ID)
		results[i].SessionID = CleanString(results[i].SessionID)
		results[i].TaskID = CleanString(results[i].TaskID)
		results[i].MissionID = CleanString(results[i].MissionID)
		results[i].MissionName = CleanString(results[i].MissionName)
		results[i].CWD = CleanString(results[i].CWD)
		results[i].Prompt = CleanString(results[i].Prompt)
		results[i].Mode = CleanString(results[i].Mode)
		results[i].Summary = CleanString(results[i].Summary)
	}
}

func sanitizeSession(session *Session) {
	session.ID = CleanString(session.ID)
	session.CWD = CleanString(session.CWD)
	session.LogPath = CleanString(session.LogPath)
	session.Title = CleanString(session.Title)
	session.LastPrompt = CleanString(session.LastPrompt)
	session.ProcessUUID = CleanString(session.ProcessUUID)
	session.InternalReason = CleanString(session.InternalReason)
	for i := range session.Events {
		sanitizeEvent(&session.Events[i])
	}
	for i := range session.Trace {
		sanitizeTraceSpan(&session.Trace[i])
	}
}

func sanitizeEvent(event *Event) {
	event.Text = CleanString(event.Text)
	event.Detail = CleanString(event.Detail)
}

func sanitizeTraceSpan(span *TraceSpan) {
	span.ID = CleanString(span.ID)
	span.ParentID = CleanString(span.ParentID)
	span.Name = CleanString(span.Name)
	span.Summary = CleanString(span.Summary)
	span.Detail = CleanString(span.Detail)
	for i := range span.Children {
		sanitizeTraceSpan(&span.Children[i])
	}
}

func sanitizeTraceRef(ref *TraceRef) {
	ref.ID = CleanString(ref.ID)
	ref.Name = CleanString(ref.Name)
	ref.Summary = CleanString(ref.Summary)
	ref.Detail = CleanString(ref.Detail)
}

func sanitizeStatusExplanation(explanation *StatusExplanation) {
	explanation.Rule = CleanString(explanation.Rule)
	explanation.RuleSeverity = CleanString(explanation.RuleSeverity)
	explanation.Reason = CleanString(explanation.Reason)
	explanation.WhyNotWorking = CleanString(explanation.WhyNotWorking)
	explanation.WhyNotIdle = CleanString(explanation.WhyNotIdle)
	if explanation.ActiveTrace != nil {
		sanitizeTraceRef(explanation.ActiveTrace)
	}
	for i := range explanation.BackgroundRunTraces {
		sanitizeTraceRef(&explanation.BackgroundRunTraces[i])
	}
	if explanation.AttentionEvent != nil {
		sanitizeEvent(explanation.AttentionEvent)
	}
	for i := range explanation.Evidence {
		explanation.Evidence[i] = CleanString(explanation.Evidence[i])
	}
}

func sanitizeDiffRadar(diff *DiffRadar) {
	diff.RepoRoot = CleanString(diff.RepoRoot)
	diff.Branch = CleanString(diff.Branch)
	diff.Summary = CleanString(diff.Summary)
	diff.LastError = CleanString(diff.LastError)
	for i := range diff.Files {
		diff.Files[i].Path = CleanString(diff.Files[i].Path)
		diff.Files[i].Status = CleanString(diff.Files[i].Status)
	}
}

func sanitizeFlightEvent(event *FlightEvent) {
	event.ID = CleanString(event.ID)
	event.ParentID = CleanString(event.ParentID)
	event.Kind = CleanString(event.Kind)
	event.Title = CleanString(event.Title)
	event.Detail = CleanString(event.Detail)
	event.Status = CleanString(event.Status)
	event.Source = CleanString(event.Source)
}

func sanitizeDebrief(debrief *Debrief) {
	debrief.TaskID = CleanString(debrief.TaskID)
	debrief.SessionID = CleanString(debrief.SessionID)
	debrief.Text = CleanString(debrief.Text)
	debrief.InputHash = CleanString(debrief.InputHash)
	debrief.Raw = CleanString(debrief.Raw)
}

func sanitizePane(pane *Pane) {
	pane.Workspace = CleanString(pane.Workspace)
	pane.Title = CleanString(pane.Title)
	pane.TabTitle = CleanString(pane.TabTitle)
	pane.WindowTitle = CleanString(pane.WindowTitle)
	pane.CWD = CleanString(pane.CWD)
	pane.TTYName = CleanString(pane.TTYName)
}

func sanitizeProcess(process *Process) {
	process.TTY = CleanString(process.TTY)
	process.Args = CleanString(process.Args)
}

func sanitizeBinding(binding *Binding) {
	if binding.Pane != nil {
		sanitizePane(binding.Pane)
	}
	if binding.Process != nil {
		sanitizeProcess(binding.Process)
	}
	for i := range binding.Reasons {
		binding.Reasons[i] = CleanString(binding.Reasons[i])
	}
}

func sanitizeMission(mission *Mission) {
	mission.ID = CleanString(mission.ID)
	mission.Name = CleanString(mission.Name)
	mission.DefaultName = CleanString(mission.DefaultName)
	mission.CWD = CleanString(mission.CWD)
	mission.RepoRoot = CleanString(mission.RepoRoot)
	mission.Summary = CleanString(mission.Summary)
	for i := range mission.TaskIDs {
		mission.TaskIDs[i] = CleanString(mission.TaskIDs[i])
	}
}

func sanitizeDoneInboxItem(item *DoneInboxItem) {
	item.ID = CleanString(item.ID)
	item.TaskID = CleanString(item.TaskID)
	item.SessionID = CleanString(item.SessionID)
	item.Project = CleanString(item.Project)
	item.CWD = CleanString(item.CWD)
	item.Title = CleanString(item.Title)
	item.Summary = CleanString(item.Summary)
	sanitizeDiffRadar(&item.Diff)
	if item.Debrief != nil {
		sanitizeDebrief(item.Debrief)
	}
}
